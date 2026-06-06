package schema

import (
	"slices"

	"github.com/eino-contrib/jsonschema"
	orderedmap "github.com/wk8/go-ordered-map/v2"
)

// DataType is the type of the parameter.
type DataType string

// Supported JSONSchema data types for tool parameters.
const (
	Object  DataType = "object"
	Number  DataType = "number"
	Integer DataType = "integer"
	String  DataType = "string"
	Array   DataType = "array"
	Null    DataType = "null"
	Boolean DataType = "boolean"
)

// ToolChoice controls how the model uses the tools provided to it.
type ToolChoice string

const (
	// ToolChoiceForbidden instructs the model not to call any tools, even if
	// tools are bound. The model responds with a plain text message instead.
	// Corresponds to "none" in OpenAI Chat Completion.
	ToolChoiceForbidden ToolChoice = "forbidden"

	// ToolChoiceAllowed lets the model decide: it may generate a plain message
	// or call one or more tools. This is the default when tools are provided.
	// Corresponds to "auto" in OpenAI Chat Completion.
	ToolChoiceAllowed ToolChoice = "allowed"

	// ToolChoiceForced requires the model to call at least one tool. Use this
	// when you want to guarantee structured output via tool calling.
	// Corresponds to "required" in OpenAI Chat Completion.
	ToolChoiceForced ToolChoice = "forced"
)

// ParameterInfo is the information of a parameter.
// It is used to describe the parameters of a tool.
type ParameterInfo struct {
	Type DataType
	// The description of the parameter.
	Desc string

	// The element type of the parameter, only for array.
	ElemInfo *ParameterInfo
	// The sub parameters of the parameter, only for object.
	SubParams map[string]*ParameterInfo
	// The enum values of the parameter, only for string.
	Enum []string
	// Whether the parameter is required.
	Required bool
}

func (p *ParameterInfo) toJSONSchema() *jsonschema.Schema {
	js := &jsonschema.Schema{
		Type:        string(p.Type),
		Description: p.Desc,
	}

	// Type: string
	if len(p.Enum) > 0 {
		js.Enum = make([]any, len(p.Enum))
		for i, enum := range p.Enum {
			js.Enum[i] = enum
		}
	}

	// Type: array
	if p.ElemInfo != nil {
		js.Items = p.ElemInfo.toJSONSchema()
	}

	// Type: object
	if len(p.SubParams) > 0 {
		required := make([]string, 0, len(p.SubParams))
		js.Properties = orderedmap.New[string, *jsonschema.Schema]()
		keys := make([]string, 0, len(p.SubParams))
		for k := range p.SubParams {
			keys = append(keys, k)
		}
		slices.Sort(keys)

		for _, k := range keys {
			v := p.SubParams[k]
			item := v.toJSONSchema()
			js.Properties.Set(k, item)
			if v.Required {
				required = append(required, k)
			}
		}

		js.Required = required
	}

	return js
}

// ParamsOneOf is a union of the different methods user can choose which describe a tool's request parameters.
// User must specify one and ONLY one method to describe the parameters.
//  1. use NewParamsOneOfByParams(): an intuitive way to describe the parameters that covers most of the use-cases.
//  2. use NewParamsOneOfByJSONSchema(): a formal way to describe the parameters that strictly adheres to JSONSchema specification.
type ParamsOneOf struct {
	params map[string]*ParameterInfo

	jsonschema *jsonschema.Schema
}

func (p *ParamsOneOf) ToJSONSchema() (*jsonschema.Schema, error) {
	if p == nil {
		return nil, nil
	}

	if p.params != nil {
		sc := &jsonschema.Schema{
			Properties: orderedmap.New[string, *jsonschema.Schema](),
			Type:       string(Object),
			Required:   make([]string, 0, len(p.params)),
		}

		keys := make([]string, 0, len(p.params))
		for k := range p.params {
			keys = append(keys, k)
		}
		slices.Sort(keys)

		for _, k := range keys {
			v := p.params[k]
			sc.Properties.Set(k, v.toJSONSchema())
			if v.Required {
				sc.Required = append(sc.Required, k)
			}
		}

		return sc, nil
	}

	return p.jsonschema, nil
}

// NewParamsOneOfByParams creates a ParamsOneOf with map[string]*ParameterInfo.
func NewParamsOneOfByParams(params map[string]*ParameterInfo) *ParamsOneOf {
	return &ParamsOneOf{
		params: params,
	}
}

// NewParamsOneOfByJSONSchema creates a ParamsOneOf with *jsonschema.Schema.
func NewParamsOneOfByJSONSchema(s *jsonschema.Schema) *ParamsOneOf {
	return &ParamsOneOf{
		jsonschema: s,
	}
}

// ToolInfo is the information of a tool.
type ToolInfo struct {
	// The unique name of the tool that clearly communicates its purpose.
	Name string
	// Used to tell the model how/when/why to use the tool.
	Desc string
	// The parameters the functions accepts.
	// can be described in two ways:
	//  - use params: schema.NewParamsOneOfByParams(params)
	//  - use jsonschema: schema.NewParamsOneOfByJSONSchema(jsonschema)
	// If is nil, signals that the tool does not need any input parameter
	*ParamsOneOf
}
