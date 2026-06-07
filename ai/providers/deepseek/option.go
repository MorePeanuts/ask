package deepseek

import (
	"github.com/MorePeanuts/ask/ai/components/model"
)

type deepseekOptions struct {
	// extraFields carries arbitrary passthrough fields that will be merged into the
	// top-level JSON payload of a chat completion request.
	//
	// It is useful when the upstream DeepSeek API introduces new request parameters
	// that are not yet modeled as first-class fields by this component (or by the
	// underlying deepseek-go SDK).
	extraFields map[string]any
}

// WithExtraFields returns a request-level option that merges the provided
// key/value pairs into the top-level JSON payload sent to the DeepSeek API.
//
// Keys that collide with fields already populated by this component (e.g.
// "model", "messages", "temperature", "thinking", ...) will override them,
// which mirrors the behavior of the underlying deepseek-go SDK.
//
// Passing a nil or empty map is a no-op.
//
// Example:
//
//	msg, err := cm.Generate(ctx, in,
//	    deepseek.WithExtraFields(map[string]interface{}{
//	        "chat_template_kwargs": map[string]interface{}{
//	            "thinking": true,
//	        },
//	    }),
//	)
func WithExtraFields(extraFields map[string]any) model.Option {
	return model.WrapImplSpecificOptFn(func(o *deepseekOptions) {
		o.extraFields = extraFields
	})
}
