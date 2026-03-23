/**
 * FieldSelection is the recursive selection object used in protograph queries.
 * An empty object {} selects a scalar field.
 * A non-empty object recurses into sub-fields (including relation fan-outs).
 */
export type FieldSelection = {
  [field: string]: FieldSelection;
};

/**
 * MethodQuery represents a single RPC call with optional request params ($)
 * and a set of response fields to select.
 */
export type MethodQuery = {
  /** "$" holds the request parameters as a plain object. */
  $?: Record<string, unknown>;
} & {
  [responseField: string]: FieldSelection;
};

/**
 * Query is the top-level protograph request body.
 *
 * @example
 * {
 *   "ng.v1.AreaService": {
 *     listAreas: {
 *       $: {},
 *       areas: { id: {}, title: {}, projects: { id: {}, title: {} } }
 *     }
 *   }
 * }
 */
export type Query = {
  [serviceName: string]: {
    [methodName: string]: MethodQuery;
  };
};
