// Package components defines common interfaces that describe component types and callback capabilities.
package components

// Component names representing the different categories of components.
type Component string

const (
	// ComponentOfChatModel identifies chat model components.
	ComponentOfChatModel Component = "ChatModel"
	// ComponentOfTool identifies tool components.
	ComponentOfTool Component = "Tool"
)
