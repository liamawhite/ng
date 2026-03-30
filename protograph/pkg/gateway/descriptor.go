package gateway

import (
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// edgeExtensionName is the fully-qualified name of the (protograph.v1alpha1.edge) extension.
const edgeExtensionName = "protograph.v1alpha1.edge"

// RelationResolver holds the metadata needed to fan-out a parent->children relation.
type RelationResolver struct {
	ServiceName        string // e.g. "ng.v1.ProjectService"
	MethodName         string // e.g. "List"
	FullMethod         string // "/ng.v1.ProjectService/List"
	RequestFilterField string // e.g. "area_id" (set per parent ID call)
	ResponseItemsField string // e.g. "projects" (repeated field in response)
	ChildMsgType       string // e.g. "ng.v1.Project"
}

// ReverseResolver holds the metadata needed to look up a child->parent relation.
type ReverseResolver struct {
	ServiceName   string // e.g. "ng.v1.AreaService"
	MethodName    string // e.g. "Get"
	FullMethod    string // "/ng.v1.AreaService/Get"
	FKField       string // proto field name on child, e.g. "area_id"
	ParentMsgType string // e.g. "ng.v1.Area"
}

// Relations holds both forward (parent->children) and reverse (child->parent) resolvers.
type Relations struct {
	// Forward: parentType -> fieldName -> RelationResolver
	Forward map[string]map[string]RelationResolver
	// Reverse: childType -> fieldName -> ReverseResolver
	Reverse map[string]map[string]ReverseResolver
}

// ExtractRelations walks message fields in the given FileDescriptors looking for
// fields annotated with (protograph.v1alpha1.edge) and builds both forward
// (target->children) and reverse (child->target) resolver maps.
//
// Forward key: parentType (e.g. "ng.v1.Area") -> fieldName (e.g. "projects")
// Reverse key: childType  (e.g. "ng.v1.Project") -> fieldName (e.g. "area")
//
// The caller must ensure the package containing the generated options.pb.go
// (which registers E_Edge) has been imported (blank import is fine).
func ExtractRelations(files []protoreflect.FileDescriptor) *Relations {
	extType, err := protoregistry.GlobalTypes.FindExtensionByName(edgeExtensionName)
	if err != nil {
		return &Relations{
			Forward: make(map[string]map[string]RelationResolver),
			Reverse: make(map[string]map[string]ReverseResolver),
		}
	}

	allFiles := resolveFileDescriptors(files)

	// Build service index: fully-qualified service name -> ServiceDescriptor.
	svcIndex := make(map[string]protoreflect.ServiceDescriptor)
	for _, fd := range allFiles {
		for i := 0; i < fd.Services().Len(); i++ {
			svc := fd.Services().Get(i)
			svcIndex[string(svc.FullName())] = svc
		}
	}

	rels := &Relations{
		Forward: make(map[string]map[string]RelationResolver),
		Reverse: make(map[string]map[string]ReverseResolver),
	}
	for _, fd := range allFiles {
		extractFromMessages(fd.Messages(), extType, svcIndex, rels)
	}
	return rels
}

func extractFromMessages(msgs protoreflect.MessageDescriptors, extType protoreflect.ExtensionType, svcIndex map[string]protoreflect.ServiceDescriptor, rels *Relations) {
	for i := 0; i < msgs.Len(); i++ {
		msg := msgs.Get(i)
		for j := 0; j < msg.Fields().Len(); j++ {
			field := msg.Fields().Get(j)
			opts := field.Options()
			if opts == nil || !proto.HasExtension(opts, extType) {
				continue
			}
			val := proto.GetExtension(opts, extType)
			ref := extractEdgeRef(val)
			if ref == nil || ref.targetType == "" {
				continue
			}

			childTypeName := string(msg.Name())         // e.g. "Project"
			childTypeFullName := string(msg.FullName()) // e.g. "ng.v1.Project"
			pkg := string(msg.ParentFile().Package())   // e.g. "ng.v1"

			// via defaults to the annotated field name if not explicitly set.
			filterField := ref.via
			if filterField == "" {
				filterField = string(field.Name()) // e.g. "area_id"
			}

			// --- Forward resolver: targetType -> children ---
			childSvcFullName := ref.listService
			if childSvcFullName == "" {
				childSvcFullName = pkg + "." + childTypeName + "Service"
			}
			listMethodName := ref.listMethod
			if listMethodName == "" {
				listMethodName = "List"
			}
			if childSvc, ok := svcIndex[childSvcFullName]; ok {
				if listMethod := childSvc.Methods().ByName(protoreflect.Name(listMethodName)); listMethod != nil {
					responseItemsField := findRepeatedField(listMethod.Output(), protoreflect.FullName(childTypeFullName))
					if responseItemsField != "" {
						forwardFieldName := strings.ToLower(childTypeName) + "s" // e.g. "projects"
						if rels.Forward[ref.targetType] == nil {
							rels.Forward[ref.targetType] = make(map[string]RelationResolver)
						}
						rels.Forward[ref.targetType][forwardFieldName] = RelationResolver{
							ServiceName:        childSvcFullName,
							MethodName:         listMethodName,
							FullMethod:         "/" + childSvcFullName + "/" + listMethodName,
							RequestFilterField: filterField,
							ResponseItemsField: responseItemsField,
							ChildMsgType:       childTypeFullName,
						}
					}
				}
			}

			// --- Reverse resolver: childType -> target ---
			// Derive the target service from the target type name.
			// e.g. "ng.v1.Area" -> pkg "ng.v1", simple "Area" -> "ng.v1.AreaService"
			lastDot := strings.LastIndex(ref.targetType, ".")
			if lastDot >= 0 {
				targetPkg := ref.targetType[:lastDot]
				targetSimpleName := ref.targetType[lastDot+1:]
				targetSvcFullName := ref.getService
				if targetSvcFullName == "" {
					targetSvcFullName = targetPkg + "." + targetSimpleName + "Service"
				}
				getMethodName := ref.getMethod
				if getMethodName == "" {
					getMethodName = "Get"
				}
				if targetSvc, ok := svcIndex[targetSvcFullName]; ok {
					if getMethod := targetSvc.Methods().ByName(protoreflect.Name(getMethodName)); getMethod != nil {
						reverseFieldName := strings.ToLower(targetSimpleName) // e.g. "area"
						if rels.Reverse[childTypeFullName] == nil {
							rels.Reverse[childTypeFullName] = make(map[string]ReverseResolver)
						}
						rels.Reverse[childTypeFullName][reverseFieldName] = ReverseResolver{
							ServiceName:   targetSvcFullName,
							MethodName:    getMethodName,
							FullMethod:    "/" + targetSvcFullName + "/" + getMethodName,
							FKField:       string(field.Name()), // always the actual field name
							ParentMsgType: ref.targetType,
						}
					}
				}
			}
		}
		// Recurse into nested message types.
		extractFromMessages(msg.Messages(), extType, svcIndex, rels)
	}
}

// findRepeatedField returns the proto field name of the first repeated message
// field in resp whose element type matches childFullName.
func findRepeatedField(resp protoreflect.MessageDescriptor, childFullName protoreflect.FullName) string {
	for i := 0; i < resp.Fields().Len(); i++ {
		fd := resp.Fields().Get(i)
		if fd.Cardinality() != protoreflect.Repeated {
			continue
		}
		if fd.Kind() != protoreflect.MessageKind {
			continue
		}
		if fd.Message().FullName() == childFullName {
			return string(fd.Name())
		}
	}
	return ""
}

type edgeRef struct {
	targetType  string
	via         string
	listService string
	listMethod  string
	getService  string
	getMethod   string
}

func extractEdgeRef(v any) *edgeRef {
	msg, ok := v.(protoreflect.ProtoMessage)
	if !ok {
		return nil
	}
	m := msg.ProtoReflect()
	fields := m.Descriptor().Fields()
	get := func(name string) string {
		fd := fields.ByName(protoreflect.Name(name))
		if fd == nil {
			return ""
		}
		return m.Get(fd).String()
	}
	return &edgeRef{
		targetType:  get("type"),
		via:         get("via"),
		listService: get("list_service"),
		listMethod:  get("list_method"),
		getService:  get("get_service"),
		getMethod:   get("get_method"),
	}
}
