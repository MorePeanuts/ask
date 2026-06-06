package model

import (
	"github.com/MorePeanuts/ask/ai/schema"
)

// Options is the common options for the model.
type Options struct {
	// ModelName is the model name.
	ModelName *string
	// Temperature is the temperature for the model, which controls the randomness of the model.
	Temperature *float32
	// TopP is the top p for the model, which controls the diversity of the model.
	TopP *float32
	// Tools is a list of tools the model may call.
	Tools []*schema.ToolInfo
	// MaxTokens is the max number of tokens, if reached the max tokens, the model will stop generating,
	// and mostly return a finish reason of "length".
	MaxTokens *int
	// StopWords is the stop words for the model, which controls the stopping condition of the model.
	StopWords []string

	// Options only available for chat model.

	// ToolChoice controls which tool is called by the model.
	ToolChoice *schema.ToolChoice
	// AllowedToolNames specifies a list of tool names that the model is allowed to call.
	// This allows for constraining the model to a specific subset of the available tools.
	AllowedToolNames []string
}

// Option is a call-time option for a ChatModel. Options are immutable and
// composable: each Option carries either a common-option setter (applied via
// [GetCommonOptions]) or an implementation-specific setter (applied via
// [GetImplSpecificOptions]), never both.
type Option struct {
	commonOptFn func(opts *Options)

	implSpecificOptFn any
}

// GetCommonOptions extracts standard [Options] from an Option list, merging
// them onto base. If base is nil, a zero-value Options is used.
//
// Implementors must call this to honour options passed by callers:
//
//	func (m *MyModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
//	    options := model.GetCommonOptions(&model.Options{Temperature: &m.defaultTemp}, opts...)
//	    // use options.Temperature, options.Tools, etc.
//	}
func GetCommonOptions(base *Options, opts ...Option) *Options {
	if base == nil {
		base = &Options{}
	}
	for _, opt := range opts {
		if opt.commonOptFn != nil {
			opt.commonOptFn(base)
		}
	}
	return base
}

// GetImplSpecificOptions extracts implementation-specific options from an
// Option list, merging them onto base. If base is nil, a zero-value T is used.
//
// Call this alongside [GetCommonOptions] to support both standard and custom
// options in your implementation:
//
//	type MyOptions struct { MyParam string }
//
//	func (m *MyModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
//	    common  := model.GetCommonOptions(nil, opts...)
//	    myOpts  := model.GetImplSpecificOptions(&MyOptions{MyParam: "default"}, opts...)
//	    // use common.Temperature, myOpts.MyParam, etc.
//	}
func GetImplSpecificOptions[T any](base *T, opts ...Option) *T {
	if base == nil {
		base = new(T)
	}
	for _, opt := range opts {
		if opt.implSpecificOptFn != nil {
			if optFn, ok := opt.implSpecificOptFn.(func(*T)); ok {
				optFn(base)
			}
		}
	}
	return base
}

// WrapImplSpecificOptFn is the option to wrap the implementation specific option function.
// WrapImplSpecificOptFn wraps an implementation-specific option function into
// an [Option] so it can be passed alongside standard options.
//
// This is intended for ChatModel implementors, not callers. Define a typed
// setter for your own config struct and expose it as an Option:
//
//	// In your implementation package:
//	func WithMyParam(v string) model.Option {
//	    return model.WrapImplSpecificOptFn(func(o *MyOptions) {
//	        o.MyParam = v
//	    })
//	}
//
// Callers can then mix standard and implementation-specific options freely:
//
//	model.Generate(ctx, msgs,
//	    model.WithTemperature(0.7),
//	    mypkg.WithMyParam("value"),
//	)
func WrapImplSpecificOptFn[T any](optFn func(*T)) Option {
	return Option{
		implSpecificOptFn: optFn,
	}
}

// WithModelName is the option to set the model name.
func WithModelName(name string) Option {
	return Option{
		commonOptFn: func(opts *Options) {
			opts.ModelName = &name
		},
	}
}

// WithTemperature is the option to set the temperature for the model.
func WithTemperature(temperature float32) Option {
	return Option{
		commonOptFn: func(opts *Options) {
			opts.Temperature = &temperature
		},
	}
}

// WithTopP is the option to set the top p for the model.
func WithTopP(topP float32) Option {
	return Option{
		commonOptFn: func(opts *Options) {
			opts.TopP = &topP
		},
	}
}

// WithTools is the option to set tools for the model.
func WithTools(tools []*schema.ToolInfo) Option {
	if tools == nil {
		tools = []*schema.ToolInfo{}
	}
	return Option{
		commonOptFn: func(opts *Options) {
			opts.Tools = tools
		},
	}
}

// WithMaxTokens is the option to set the max tokens for the model.
func WithMaxTokens(maxTokens int) Option {
	return Option{
		commonOptFn: func(opts *Options) {
			opts.MaxTokens = &maxTokens
		},
	}
}

// WithStopWords is the option to set the stop words for the model.
func WithStopWords(stopWords []string) Option {
	return Option{
		commonOptFn: func(opts *Options) {
			opts.StopWords = stopWords
		},
	}
}

// WithToolChoice sets the tool choice for the model. It also allows for providing a list of
// tool names to constrain the model to a specific subset of the available tools.
// Only available for ChatModel.
func WithToolChoice(toolChoice schema.ToolChoice, allowedToolNames ...string) Option {
	return Option{
		commonOptFn: func(opts *Options) {
			opts.ToolChoice = &toolChoice
			opts.AllowedToolNames = allowedToolNames
		},
	}
}
