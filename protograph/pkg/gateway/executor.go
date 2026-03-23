package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// executor executes a parsed query against gRPC and stitches results.
type executor struct {
	conn             *grpc.ClientConn
	relations        map[string]map[string]RelationResolver
	reverseRelations map[string]map[string]ReverseResolver
	// msgTypes: fully-qualified message name → MessageDescriptor
	msgTypes map[string]protoreflect.MessageDescriptor
}

func newExecutor(conn *grpc.ClientConn, rels *Relations, files []protoreflect.FileDescriptor) *executor {
	msgTypes := make(map[string]protoreflect.MessageDescriptor)
	for _, fd := range files {
		walkMessages(fd.Messages(), msgTypes)
	}
	return &executor{conn: conn, relations: rels.Forward, reverseRelations: rels.Reverse, msgTypes: msgTypes}
}

func walkMessages(msgs protoreflect.MessageDescriptors, out map[string]protoreflect.MessageDescriptor) {
	for i := 0; i < msgs.Len(); i++ {
		m := msgs.Get(i)
		out[string(m.FullName())] = m
		walkMessages(m.Messages(), out)
	}
}

// invokeResult holds the output of a single entry-point RPC call.
type invokeResult struct {
	svcName    string
	methodName string // original query name (may be lowerCamelCase)
	masked     map[string]any
	err        error
}

// execute runs all service/method calls in the query concurrently and returns the stitched result.
func (e *executor) execute(ctx context.Context, q Query, filesByService map[string]serviceInfo) (map[string]any, error) {
	// Collect all (service, method) pairs first to validate before launching goroutines.
	type invocation struct {
		svcName    string
		methodName string // original query name
		protoName  string // PascalCase proto RPC name
		mq         MethodQuery
		methodDesc protoreflect.MethodDescriptor
	}
	var invocations []invocation
	for svcName, methods := range q {
		info, ok := filesByService[svcName]
		if !ok {
			return nil, fmt.Errorf("unknown service %q", svcName)
		}
		for methodName, mq := range methods {
			// Accept lowerCamelCase names ("listAreas") by promoting the first letter.
			// "listAreas" → "ListAreas", "List" → "List".
			protoName := upperFirst(methodName)
			methodDesc := info.svc.Methods().ByName(protoreflect.Name(protoName))
			if methodDesc == nil {
				return nil, fmt.Errorf("method %q not found on %q", methodName, svcName)
			}
			invocations = append(invocations, invocation{
				svcName:    svcName,
				methodName: methodName,
				protoName:  protoName,
				mq:         mq,
				methodDesc: methodDesc,
			})
		}
	}

	// Dispatch all entry-point RPCs concurrently.
	results := make(chan invokeResult, len(invocations))
	var wg sync.WaitGroup
	for _, inv := range invocations {
		wg.Add(1)
		go func(inv invocation) {
			defer wg.Done()
			masked, err := e.invokeMethod(ctx, inv.svcName, inv.protoName, inv.methodDesc, inv.mq)
			results <- invokeResult{
				svcName:    inv.svcName,
				methodName: inv.methodName,
				masked:     masked,
				err:        err,
			}
		}(inv)
	}
	wg.Wait()
	close(results)

	// Collect results. Return the first error encountered.
	// svcResults: service → method → masked response
	svcResults := make(map[string]map[string]any)
	for r := range results {
		if r.err != nil {
			return nil, r.err
		}
		if svcResults[r.svcName] == nil {
			svcResults[r.svcName] = make(map[string]any)
		}
		// Key by original query name so the response mirrors what the client sent.
		svcResults[r.svcName][r.methodName] = r.masked
	}

	// Promote svcResults to the top-level result map.
	result := make(map[string]any, len(svcResults))
	for svc, methods := range svcResults {
		result[svc] = methods
	}
	return result, nil
}

// invokeMethod executes a single RPC and returns the masked, stitched response.
func (e *executor) invokeMethod(ctx context.Context, svcName, protoName string, methodDesc protoreflect.MethodDescriptor, mq MethodQuery) (map[string]any, error) {
	reqMsg, err := buildRequest(methodDesc.Input(), mq.Params)
	if err != nil {
		return nil, fmt.Errorf("build request for %s/%s: %w", svcName, protoName, err)
	}

	respDesc := methodDesc.Output()
	respMsg := dynamicpb.NewMessage(respDesc)
	fullMethod := "/" + svcName + "/" + protoName
	if err := e.conn.Invoke(ctx, fullMethod, reqMsg, respMsg); err != nil {
		return nil, fmt.Errorf("invoke %s: %w", fullMethod, err)
	}

	marshaller := protojson.MarshalOptions{EmitUnpopulated: false, UseProtoNames: false}
	jsonBytes, err := marshaller.Marshal(respMsg)
	if err != nil {
		return nil, fmt.Errorf("marshal %s response: %w", fullMethod, err)
	}
	var respMap map[string]any
	if err := json.Unmarshal(jsonBytes, &respMap); err != nil {
		return nil, fmt.Errorf("unmarshal %s response: %w", fullMethod, err)
	}

	masked, err := e.maskAndFanOut(ctx, respMap, mq.Fields, string(respDesc.FullName()))
	if err != nil {
		return nil, fmt.Errorf("mask/fanout %s: %w", fullMethod, err)
	}
	return masked, nil
}

// maskAndFanOut applies field mask and fans out relations at each level of the response.
func (e *executor) maskAndFanOut(ctx context.Context, m map[string]any, sel FieldSelection, msgTypeName string) (map[string]any, error) {
	result := make(map[string]any, len(sel))

	for fieldKey, subSel := range sel {
		camel := toCamelCase(fieldKey)
		val, ok := m[camel]
		if !ok {
			val, ok = m[fieldKey]
		}
		if !ok {
			continue
		}

		switch typed := val.(type) {
		case map[string]any:
			if len(subSel) == 0 {
				result[camel] = typed
			} else {
				nested, err := e.maskAndFanOut(ctx, typed, subSel, "")
				if err != nil {
					return nil, err
				}
				result[camel] = nested
			}
		case []any:
			items := make([]map[string]any, 0, len(typed))
			for _, item := range typed {
				if itemMap, ok := item.(map[string]any); ok {
					items = append(items, itemMap)
				}
			}

			// Determine the element message type for relation fan-out.
			// First try the proto descriptor; fall back to the resolver's ChildMsgType
			// for fields that were synthetically added by relation stitching (e.g.
			// "projects" on Area doesn't exist in the proto, but is added by the resolver).
			elemType := e.fieldElemType(msgTypeName, fieldKey)
			if elemType == "" {
				elemType = e.childMsgTypeFromResolver(msgTypeName, fieldKey)
			}
			if err := e.fanOutRelations(ctx, items, elemType, subSel); err != nil {
				return nil, err
			}
			if err := e.fanOutReverseRelations(ctx, items, elemType, subSel); err != nil {
				return nil, err
			}

			// Recursively mask and fan-out each item (handles nested relations, e.g. tasks inside projects).
			maskedItems := make([]any, 0, len(items))
			for _, item := range items {
				masked, err := e.maskAndFanOut(ctx, item, subSel, elemType)
				if err != nil {
					return nil, err
				}
				maskedItems = append(maskedItems, masked)
			}
			result[camel] = maskedItems

		default:
			result[camel] = val
		}
	}
	return result, nil
}

// fanOutRelations takes a list of parent items (each a map), looks up all relation
// resolvers for parentMsgType, and for each requested relation field, calls List
// once per parent ID in parallel and stitches results back onto the parent maps.
func (e *executor) fanOutRelations(ctx context.Context, items []map[string]any, parentMsgType string, sel FieldSelection) error {
	resolvers, ok := e.relations[parentMsgType]
	if !ok {
		return nil
	}

	for fieldName, resolver := range resolvers {
		subSel, wantRelation := sel[fieldName]
		if !wantRelation {
			continue
		}

		// Collect parent IDs (the "id" field on each parent item).
		parentIDs := make([]string, 0, len(items))
		for _, item := range items {
			if id, _ := item["id"].(string); id != "" {
				parentIDs = append(parentIDs, id)
			}
		}
		if len(parentIDs) == 0 {
			continue
		}

		resolverMethod, err := e.findMethod(resolver.ServiceName, resolver.MethodName)
		if err != nil {
			return fmt.Errorf("find resolver method %s: %w", resolver.FullMethod, err)
		}

		// Call List once per parent ID in parallel.
		type perParent struct {
			id    string
			items []map[string]any
			err   error
		}
		ch := make(chan perParent, len(parentIDs))
		var wg sync.WaitGroup
		for _, pid := range parentIDs {
			wg.Add(1)
			go func(pid string) {
				defer wg.Done()
				reqMsg, err := buildRequest(resolverMethod.Input(), map[string]any{
					resolver.RequestFilterField: pid,
				})
				if err != nil {
					ch <- perParent{id: pid, err: fmt.Errorf("build request: %w", err)}
					return
				}
				respMsg := dynamicpb.NewMessage(resolverMethod.Output())
				if err := e.conn.Invoke(ctx, resolver.FullMethod, reqMsg, respMsg); err != nil {
					ch <- perParent{id: pid, err: fmt.Errorf("invoke %s: %w", resolver.FullMethod, err)}
					return
				}
				marshaller := protojson.MarshalOptions{EmitUnpopulated: false, UseProtoNames: false}
				jsonBytes, err := marshaller.Marshal(respMsg)
				if err != nil {
					ch <- perParent{id: pid, err: fmt.Errorf("marshal: %w", err)}
					return
				}
				var respMap map[string]any
				if err := json.Unmarshal(jsonBytes, &respMap); err != nil {
					ch <- perParent{id: pid, err: fmt.Errorf("unmarshal: %w", err)}
					return
				}
				itemsKey := toCamelCase(resolver.ResponseItemsField)
				rawItems, _ := respMap[itemsKey].([]any)
				children := make([]map[string]any, 0, len(rawItems))
				for _, raw := range rawItems {
					if child, ok := raw.(map[string]any); ok {
						children = append(children, child)
					}
				}
				ch <- perParent{id: pid, items: children}
			}(pid)
		}
		wg.Wait()
		close(ch)

		// Collect results and build bucket: parentID → []masked child.
		bucket := make(map[string][]map[string]any, len(parentIDs))
		for r := range ch {
			if r.err != nil {
				return r.err
			}
			for _, child := range r.items {
				var masked map[string]any
				if len(subSel) > 0 {
					masked = make(map[string]any, len(subSel))
					for sk := range subSel {
						ck := toCamelCase(sk)
						if v, ok := child[ck]; ok {
							masked[ck] = v
						} else if v, ok := child[sk]; ok {
							masked[sk] = v
						}
					}
				} else {
					masked = child
				}
				bucket[r.id] = append(bucket[r.id], masked)
			}
		}

		// Stitch children onto parent items.
		for _, item := range items {
			id, _ := item["id"].(string)
			children := bucket[id]
			if children == nil {
				children = []map[string]any{}
			}
			childrenAny := make([]any, len(children))
			for i, c := range children {
				childrenAny[i] = c
			}
			item[fieldName] = childrenAny
		}
	}
	return nil
}

// fanOutReverseRelations takes a list of child items (each a map), looks up
// reverse resolvers for childMsgType, and for each requested parent field calls
// Get once per unique FK value in parallel, then stitches the parent map onto
// each child item.
func (e *executor) fanOutReverseRelations(ctx context.Context, items []map[string]any, childMsgType string, sel FieldSelection) error {
	resolvers, ok := e.reverseRelations[childMsgType]
	if !ok {
		return nil
	}

	for fieldName, resolver := range resolvers {
		if _, wantRelation := sel[fieldName]; !wantRelation {
			continue
		}

		// Collect unique FK values from items (e.g. area_id on each Project).
		fkCamel := toCamelCase(resolver.FKField)
		fkSet := make(map[string]struct{})
		for _, item := range items {
			if fk, _ := item[fkCamel].(string); fk != "" {
				fkSet[fk] = struct{}{}
			}
		}
		if len(fkSet) == 0 {
			continue
		}

		resolverMethod, err := e.findMethod(resolver.ServiceName, resolver.MethodName)
		if err != nil {
			return fmt.Errorf("find reverse resolver method %s: %w", resolver.FullMethod, err)
		}

		// Call Get once per unique FK value in parallel.
		type perParent struct {
			id     string
			parent map[string]any
			err    error
		}
		ch := make(chan perParent, len(fkSet))
		var wg sync.WaitGroup
		for fk := range fkSet {
			wg.Add(1)
			go func(fk string) {
				defer wg.Done()
				reqMsg, err := buildRequest(resolverMethod.Input(), map[string]any{"id": fk})
				if err != nil {
					ch <- perParent{id: fk, err: fmt.Errorf("build request: %w", err)}
					return
				}
				respMsg := dynamicpb.NewMessage(resolverMethod.Output())
				if err := e.conn.Invoke(ctx, resolver.FullMethod, reqMsg, respMsg); err != nil {
					ch <- perParent{id: fk, err: fmt.Errorf("invoke %s: %w", resolver.FullMethod, err)}
					return
				}
				marshaller := protojson.MarshalOptions{EmitUnpopulated: false, UseProtoNames: false}
				jsonBytes, err := marshaller.Marshal(respMsg)
				if err != nil {
					ch <- perParent{id: fk, err: fmt.Errorf("marshal: %w", err)}
					return
				}
				var parentMap map[string]any
				if err := json.Unmarshal(jsonBytes, &parentMap); err != nil {
					ch <- perParent{id: fk, err: fmt.Errorf("unmarshal: %w", err)}
					return
				}
				ch <- perParent{id: fk, parent: parentMap}
			}(fk)
		}
		wg.Wait()
		close(ch)

		// Collect results: fk → parent map.
		parentByFK := make(map[string]map[string]any, len(fkSet))
		for r := range ch {
			if r.err != nil {
				return r.err
			}
			parentByFK[r.id] = r.parent
		}

		// Stitch parent onto each item.
		for _, item := range items {
			fk, _ := item[fkCamel].(string)
			if fk == "" {
				continue
			}
			if parent, ok := parentByFK[fk]; ok {
				item[fieldName] = parent
			}
		}
	}
	return nil
}

// childMsgTypeFromResolver returns the ChildMsgType from a registered resolver,
// used for fields that exist in the response due to relation stitching but are not
// declared in the proto message itself.
func (e *executor) childMsgTypeFromResolver(parentMsgType, fieldName string) string {
	if resolvers, ok := e.relations[parentMsgType]; ok {
		if resolver, ok := resolvers[fieldName]; ok {
			return resolver.ChildMsgType
		}
	}
	return ""
}

// fieldElemType returns the fully-qualified message type of elements in a repeated
// field, given the parent message type name and field name.
func (e *executor) fieldElemType(msgTypeName, fieldName string) string {
	if msgTypeName == "" {
		return ""
	}
	desc, ok := e.msgTypes[msgTypeName]
	if !ok {
		return ""
	}
	fd := desc.Fields().ByName(protoreflect.Name(fieldName))
	if fd == nil {
		fd = desc.Fields().ByJSONName(fieldName)
	}
	if fd == nil {
		return ""
	}
	if fd.Kind() == protoreflect.MessageKind {
		return string(fd.Message().FullName())
	}
	return ""
}

// findMethod looks up the MethodDescriptor across all registered files.
func (e *executor) findMethod(svcName, methodName string) (protoreflect.MethodDescriptor, error) {
	for _, desc := range e.msgTypes {
		fd := desc.ParentFile()
		if fd == nil {
			continue
		}
		for i := 0; i < fd.Services().Len(); i++ {
			svc := fd.Services().Get(i)
			if string(svc.FullName()) != svcName {
				continue
			}
			m := svc.Methods().ByName(protoreflect.Name(methodName))
			if m != nil {
				return m, nil
			}
		}
	}
	return nil, fmt.Errorf("method %s/%s not found", svcName, methodName)
}

// buildRequest constructs a dynamic proto message from a params map.
func buildRequest(desc protoreflect.MessageDescriptor, params map[string]any) (*dynamicpb.Message, error) {
	msg := dynamicpb.NewMessage(desc)
	if len(params) == 0 {
		return msg, nil
	}
	jsonBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(jsonBytes, msg); err != nil {
		return nil, fmt.Errorf("unmarshal params into %s: %w", desc.FullName(), err)
	}
	return msg, nil
}

// upperFirst upper-cases the first rune of s, leaving the rest unchanged.
// Used to promote lowerCamelCase query method names to proto PascalCase names.
func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// toCamelCase converts proto_snake_case to protoCamelCase (lowerCamelCase).
func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) == 1 {
		return s
	}
	var b strings.Builder
	for i, p := range parts {
		if i == 0 {
			b.WriteString(p)
			continue
		}
		if len(p) == 0 {
			continue
		}
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

// serviceInfo bundles a ServiceDescriptor with its file.
type serviceInfo struct {
	svc protoreflect.ServiceDescriptor
	fd  protoreflect.FileDescriptor
}

// buildServiceIndex builds the map from service full name to serviceInfo.
func buildServiceIndex(files []protoreflect.FileDescriptor) map[string]serviceInfo {
	idx := make(map[string]serviceInfo)
	for _, fd := range files {
		for i := 0; i < fd.Services().Len(); i++ {
			svc := fd.Services().Get(i)
			idx[string(svc.FullName())] = serviceInfo{svc: svc, fd: fd}
		}
		// Walk direct imports so services from imported files are accessible.
		imports := fd.Imports()
		for j := 0; j < imports.Len(); j++ {
			dep := imports.Get(j)
			for k := 0; k < dep.Services().Len(); k++ {
				svc := dep.Services().Get(k)
				idx[string(svc.FullName())] = serviceInfo{svc: svc, fd: dep}
			}
		}
	}
	return idx
}

// resolveFileDescriptors returns all FileDescriptors reachable from the given
// roots, via transitive imports.
func resolveFileDescriptors(files []protoreflect.FileDescriptor) []protoreflect.FileDescriptor {
	seen := make(map[string]bool)
	var result []protoreflect.FileDescriptor
	var walk func(fd protoreflect.FileDescriptor)
	walk = func(fd protoreflect.FileDescriptor) {
		name := string(fd.Path())
		if seen[name] {
			return
		}
		seen[name] = true
		result = append(result, fd)
		imports := fd.Imports()
		for i := 0; i < imports.Len(); i++ {
			walk(imports.Get(i))
		}
	}
	for _, fd := range files {
		walk(fd)
	}
	_ = protodesc.ToFileDescriptorProto // ensure import is used
	return result
}
