// Data types for the Meta AI request context passed to handler functions.
package main

import "go.mau.fi/whatsmeow/types"

// Data contains the full context for a Meta AI request, including chat
// metadata, user info, quoted messages, and group state.
type Data struct {
	ChatID                       string          `json:"chat_id"`
	Question                     string          `json:"question"`
	MessageID                    string          `json:"message_id"`
	User                         types.JID       `json:"user"`
	PushName                     string          `json:"push_name"`
	Language                     string          `json:"language,omitempty"`
	QuotedMessageID              string          `json:"quoted_message_id"`
	QuotedMessageOfQuestion      string          `json:"quoted_message_of_question"`
	QuotedMessageType            string          `json:"quoted_message_type"`
	QuotedImageBase64            string          `json:"quoted_image_base64"`
	QuotedImageMimeType          string          `json:"quoted_image_mime_type"`
	UserOfQuotedMessage          string          `json:"user_of_quoted_message"`
	QuotedMessageParticipantRole string          `json:"quoted_message_participant_role"`
	Role                         string          `json:"role"`
	ChatType                     string          `json:"chat_type"`
	IsSudo                       bool            `json:"is_sudo"`
	GroupMetaData                types.GroupInfo `json:"group_meta_data"`
	ConversationHistory          string          `json:"conversation_history,omitempty"`
}

// Tools describes which tools the AI may invoke in its response.
type Tools struct {
	Shell   string `json:"shell"`   // Danger
	Command string `json:"command"` // name of a registered bot command to invoke, e.g. "cpu"
}

// MetaResponse is the structured reply from Meta AI, containing the answer
// text and any tool invocations to execute.
type MetaResponse struct {
	ChatID string `json:"chat_id"`
	Answer string `json:"answer"`
	Tools  Tools  `json:"tools"`
}
