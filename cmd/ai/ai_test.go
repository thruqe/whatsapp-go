package ai

import (
	"strings"
	"testing"

	"whatsrook/cmd/dispatch"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waAICommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestRenderGroupContext(t *testing.T) {
	// 1. Empty group info should return empty string
	emptyInfo := types.GroupInfo{}
	if got := RenderGroupContext(emptyInfo); got != "" {
		t.Errorf("expected empty string for empty GroupInfo, got: %q", got)
	}

	// 2. Normal group info
	info := types.GroupInfo{
		GroupName: types.GroupName{
			Name: "Engineers Club",
		},
		GroupTopic: types.GroupTopic{
			Topic: "Discussion of Go, WhatsApp bot architectures, and high-performance protocols.",
		},
		ParticipantCount: 42,
	}
	rendered := RenderGroupContext(info)
	if !strings.Contains(rendered, "[GROUP CONTEXT]") {
		t.Errorf("expected [GROUP CONTEXT] header, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Group Name: Engineers Club") {
		t.Errorf("expected group name in context, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Participant Count: 42") {
		t.Errorf("expected participant count in context, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Group Description:") {
		t.Errorf("expected group description in context, got: %s", rendered)
	}
	if !strings.Contains(rendered, "[/GROUP CONTEXT]") {
		t.Errorf("expected [/GROUP CONTEXT] footer, got: %s", rendered)
	}

	// 3. Topic truncation test (> 150 chars)
	longTopic := strings.Repeat("A", 160)
	infoLong := types.GroupInfo{
		GroupName: types.GroupName{Name: "Test Group"},
		GroupTopic: types.GroupTopic{
			Topic: longTopic,
		},
		ParticipantCount: 5,
	}
	renderedLong := RenderGroupContext(infoLong)
	if !strings.Contains(renderedLong, "...") {
		t.Errorf("expected truncated topic to contain ellipsis, got: %s", renderedLong)
	}
	if strings.Contains(renderedLong, longTopic) {
		t.Errorf("expected topic exceeding 150 chars to be truncated, got: %s", renderedLong)
	}
}

func TestRenderUserContext(t *testing.T) {
	// 1. User with PushName and Sudo
	data := Data{
		PushName:  "『𖥠』ємρєяσя{ 𝕱𝖆𝖙𝖍𝖊𝖗 𝖔𝖋 𝕷𝖔𝖗𝖉𝖘 }",
		IsSudo:    true,
		MessageID: "A5D99E69EB069EF99612CE31F379C2CB",
		User:      types.NewJID("1234567890", types.DefaultUserServer),
	}
	rendered := RenderUserContext(data)

	if !strings.Contains(rendered, "[USER CONTEXT]") {
		t.Errorf("expected [USER CONTEXT] header, got: %s", rendered)
	}
	if !strings.Contains(rendered, "User: 『𖥠』ємρєяσя{ 𝕱𝖆𝖙𝖍𝖊𝖗 𝖔𝖋 𝕷𝖔𝖗𝖉𝖘 }") {
		t.Errorf("expected push name in user context, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Status: Owner/Sudo") {
		t.Errorf("expected sudo status in context, got: %s", rendered)
	}
	// Verify raw message ID and raw JID are NOT leaked
	if strings.Contains(rendered, "A5D99E69EB069EF99612CE31F379C2CB") {
		t.Errorf("raw message ID leaked in user context: %s", rendered)
	}
	if strings.Contains(rendered, "1234567890") {
		t.Errorf("raw user JID leaked in user context: %s", rendered)
	}
	if !strings.Contains(rendered, "Address the user in conversation using their name above") {
		t.Errorf("expected name instruction in context, got: %s", rendered)
	}

	// 2. User without PushName should fall back to "User"
	dataEmpty := Data{
		PushName: "",
		IsSudo:   false,
	}
	renderedEmpty := RenderUserContext(dataEmpty)
	if !strings.Contains(renderedEmpty, "User: User") {
		t.Errorf("expected fallback to 'User: User', got: %s", renderedEmpty)
	}
	if strings.Contains(renderedEmpty, "Status: Owner/Sudo") {
		t.Errorf("did not expect sudo status for non-sudo user, got: %s", renderedEmpty)
	}
}

func TestRenderQuotedContext(t *testing.T) {
	// 1. Empty quoted message returns empty string
	emptyData := Data{}
	if got := RenderQuotedContext(emptyData); got != "" {
		t.Errorf("expected empty string for empty quoted data, got: %q", got)
	}

	// 2. Populated quoted message
	data := Data{
		UserOfQuotedMessage:          "Thruqe",
		QuotedMessageParticipantRole: "Member",
		QuotedMessageType:            "Text",
		QuotedMessageOfQuestion:      "is the panel running",
		QuotedMessageID:              "3EB0F099B859956C4ECC80",
	}
	rendered := RenderQuotedContext(data)

	if !strings.Contains(rendered, "[REPLYING TO A MESSAGE — EXTRACTED CONTEXT]") {
		t.Errorf("expected header, got: %s", rendered)
	}
	if !strings.Contains(rendered, "From: Thruqe (Member)") {
		t.Errorf("expected participant and role, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Message Type: Text") {
		t.Errorf("expected message type, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Message Content: is the panel running") {
		t.Errorf("expected message content, got: %s", rendered)
	}
	// Quoted Message ID must not be leaked
	if strings.Contains(rendered, "3EB0F099B859956C4ECC80") {
		t.Errorf("quoted message ID leaked in quoted context: %s", rendered)
	}

	// 3. Truncation of long quoted message (> 500 chars)
	longMsg := strings.Repeat("M", 550)
	dataLong := Data{
		QuotedMessageType:       "Text",
		QuotedMessageOfQuestion: longMsg,
	}
	renderedLong := RenderQuotedContext(dataLong)
	if !strings.Contains(renderedLong, "...") {
		t.Errorf("expected truncated content to have ellipsis, got: %s", renderedLong)
	}
	if strings.Contains(renderedLong, longMsg) {
		t.Errorf("expected message over 500 chars to be truncated, got: %s", renderedLong)
	}
}

func TestRenderCurrentMessage(t *testing.T) {
	// 1. With question and quoted reference
	data := Data{
		PushName:                "『𖥠』ємρєяσя{ 𝕱𝖆𝖙𝖍𝖊𝖗 𝖔𝖋 𝕷𝖔𝖗𝖉𝖘 }",
		Question:                "Never stopped running",
		QuotedMessageOfQuestion: "is the panel running",
	}
	rendered := RenderCurrentMessage(data)

	if !strings.Contains(rendered, "[CURRENT MESSAGE]") {
		t.Errorf("expected header, got: %s", rendered)
	}
	if !strings.Contains(rendered, "From: 『𖥠』ємρєяσя{ 𝕱𝖆𝖙𝖍𝖊𝖗 𝖔𝖋 𝕷𝖔𝖗𝖉𝖘 }") {
		t.Errorf("expected sender name, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Message: Never stopped running") {
		t.Errorf("expected message content, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Note: The user is replying to the quoted message referenced above in this chat.") {
		t.Errorf("expected reply note, got: %s", rendered)
	}

	// 2. Empty question (user called bot on quoted message without additional text)
	dataNoQ := Data{
		PushName:                "Alice",
		Question:                "",
		QuotedMessageOfQuestion: "Please review this code",
	}
	renderedNoQ := RenderCurrentMessage(dataNoQ)
	if !strings.Contains(renderedNoQ, "Instruction: The user called the bot to respond to the quoted message above") {
		t.Errorf("expected instruction for empty question on quoted message, got: %s", renderedNoQ)
	}
}

func TestBuildAiQuery(t *testing.T) {
	instruction := "[SYSTEM INSTRUCTION]\nBot identity\n\n"
	customPrompt := "Be concise and witty."

	// 1. Standard group chat query with quoted message
	groupInfo := types.GroupInfo{
		GroupName:        types.GroupName{Name: "Core Ops"},
		ParticipantCount: 10,
	}
	data := Data{
		ChatType:                     "group",
		GroupMetaData:                groupInfo,
		PushName:                     "『𖥠』ємρєяσя{ 𝕱𝖆𝖙𝖍𝖊𝖗 𝖔𝖋 𝕷𝖔𝖗𝖉𝖘 }",
		IsSudo:                       true,
		QuotedMessageType:            "Text",
		UserOfQuotedMessage:          "Thruqe",
		QuotedMessageParticipantRole: "Member",
		QuotedMessageOfQuestion:      "is the panel running",
		Question:                     "Never stopped running",
	}

	query := BuildAiQuery(instruction, customPrompt, data)

	if !strings.Contains(query, "[SYSTEM INSTRUCTION]") {
		t.Errorf("missing system instruction: %s", query)
	}
	if !strings.Contains(query, "[GLOBAL BOT PERSONALITY & RELATIONSHIP BEHAVIOR INSTRUCTION]") {
		t.Errorf("missing custom prompt header: %s", query)
	}
	if !strings.Contains(query, "Be concise and witty.") {
		t.Errorf("missing custom prompt body: %s", query)
	}
	if !strings.Contains(query, "[GROUP CONTEXT]") {
		t.Errorf("missing group context: %s", query)
	}
	if !strings.Contains(query, "Group Name: Core Ops") {
		t.Errorf("missing group name: %s", query)
	}
	if !strings.Contains(query, "[USER CONTEXT]") {
		t.Errorf("missing user context: %s", query)
	}
	if !strings.Contains(query, "User: 『𖥠』ємρєяσя{ 𝕱𝖆𝖙𝖍𝖊𝖗 𝖔𝖋 𝕷𝖔𝖗𝖉𝖘 }") {
		t.Errorf("missing user display name: %s", query)
	}
	if !strings.Contains(query, "[REPLYING TO A MESSAGE — EXTRACTED CONTEXT]") {
		t.Errorf("missing quoted message context: %s", query)
	}
	if !strings.Contains(query, "[CURRENT MESSAGE]") {
		t.Errorf("missing current message context: %s", query)
	}
	if !strings.Contains(query, "Message: Never stopped running") {
		t.Errorf("missing current message body: %s", query)
	}

	// 2. Direct message (non-group) query
	dataDM := Data{
		ChatType: "direct",
		PushName: "Alice",
		Question: "What is quantum computing?",
	}
	queryDM := BuildAiQuery(instruction, "", dataDM)
	if strings.Contains(queryDM, "[GROUP CONTEXT]") {
		t.Errorf("direct message query should not contain group context: %s", queryDM)
	}
	if strings.Contains(queryDM, "[GLOBAL BOT PERSONALITY") {
		t.Errorf("should not contain custom prompt when empty: %s", queryDM)
	}
	if strings.Contains(queryDM, "[REPLYING TO A MESSAGE") {
		t.Errorf("should not contain quoted context when not replying: %s", queryDM)
	}
	if !strings.Contains(queryDM, "Message: What is quantum computing?") {
		t.Errorf("missing question in DM query: %s", queryDM)
	}

	// 3. Media generation prompt should bypass all context wrappers
	dataImagine := Data{
		ChatType:      "group",
		GroupMetaData: groupInfo,
		PushName:      "Alice",
		Question:      "imagine a cybernetic dragon soaring over neon skyscrapers",
	}
	queryImagine := BuildAiQuery(instruction, customPrompt, dataImagine)
	if queryImagine != dataImagine.Question {
		t.Errorf("media generation query must return raw question string, got: %q", queryImagine)
	}
}

func TestParseRunCommand(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCmdName string
		wantRawArgs string
		wantOk      bool
	}{
		{
			name:        "Standard command without args",
			input:       "RUN_COMMAND: !ping",
			wantCmdName: "ping",
			wantRawArgs: "",
			wantOk:      true,
		},
		{
			name:        "Command with args and dot prefix",
			input:       "RUN_COMMAND: .menu all",
			wantCmdName: "menu",
			wantRawArgs: "all",
			wantOk:      true,
		},
		{
			name:        "Command with multiple args",
			input:       "RUN_COMMAND: /eval fmt.Println(\"hello\")",
			wantCmdName: "eval",
			wantRawArgs: "fmt.Println(\"hello\")",
			wantOk:      true,
		},
		{
			name:        "Command wrapped in backticks",
			input:       "`RUN_COMMAND: !restart`",
			wantCmdName: "restart",
			wantRawArgs: "",
			wantOk:      true,
		},
		{
			name:        "Command containing link unavailable artifact",
			input:       "RUN_COMMAND: !weather Tokyo (link unavailable)",
			wantCmdName: "weather",
			wantRawArgs: "Tokyo",
			wantOk:      true,
		},
		{
			name:        "Command with leading whitespace and prefix",
			input:       "  RUN_COMMAND: #sudo add 1234567  ",
			wantCmdName: "sudo",
			wantRawArgs: "add 1234567",
			wantOk:      true,
		},
		{
			name:        "Normal conversational reply",
			input:       "Here is an explanation of quantum physics.",
			wantCmdName: "",
			wantRawArgs: "",
			wantOk:      false,
		},
		{
			name:        "Empty RUN_COMMAND text",
			input:       "RUN_COMMAND:   ",
			wantCmdName: "",
			wantRawArgs: "",
			wantOk:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdName, rawArgs, ok := ParseRunCommand(tt.input)
			if ok != tt.wantOk {
				t.Fatalf("ParseRunCommand(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
			if cmdName != tt.wantCmdName {
				t.Errorf("ParseRunCommand(%q) cmdName = %q, want %q", tt.input, cmdName, tt.wantCmdName)
			}
			if rawArgs != tt.wantRawArgs {
				t.Errorf("ParseRunCommand(%q) rawArgs = %q, want %q", tt.input, rawArgs, tt.wantRawArgs)
			}
		})
	}
}

func TestIsMediaGenerationPrompt(t *testing.T) {
	positiveCases := []string{
		"imagine a majestic eagle in flight",
		"/imagine sunset over ocean",
		"image of an ancient castle",
		"generate an image of a red sports car",
		"generate a photo of Paris at twilight",
		"create an image of a futuristic robot",
		"generate image of a cozy cabin",
		"create image of an astronaut on Mars",
		"draw a cute kitten sleeping in a basket",
		"picture of a tropical beach",
		"photo of an aurora borealis",
		"make an image of a cyber cafe",
		"make a picture of coffee and croissants",
		"video of running water in a brook",
		"generate a video of a spaceship launch",
		"generate video of floating balloons",
		"create a video of flowers blooming",
		"create video of clouds time lapse",
		"animate a flying butterfly",
		"make a video of a city at night",
	}

	for _, p := range positiveCases {
		if !isMediaGenerationPrompt(p) {
			t.Errorf("isMediaGenerationPrompt(%q) = false, want true", p)
		}
	}

	negativeCases := []string{
		"what is the speed of light?",
		"is the panel running",
		"explain quantum mechanics",
		"here is your code",
		"here you go with the summary",
		"what does an image sensor do in modern cameras?",
		"can you create a video script for my YouTube channel?",
		"how to draw a cartoon character on paper",
	}

	for _, p := range negativeCases {
		if isMediaGenerationPrompt(p) {
			t.Errorf("isMediaGenerationPrompt(%q) = true, want false", p)
		}
	}
}

func TestIsDummyPlaceholderText(t *testing.T) {
	dummies := []string{
		"",
		"   ",
		"_",
		"__",
		"___",
		"_We_",
		"_Thinking_",
		"...",
		"thinking...",
	}

	for _, d := range dummies {
		if !IsDummyPlaceholderText(d) {
			t.Errorf("IsDummyPlaceholderText(%q) = false, want true", d)
		}
	}

	notDummies := []string{
		"Hello world",
		"42",
		"The answer is yes.",
	}

	for _, nd := range notDummies {
		if IsDummyPlaceholderText(nd) {
			t.Errorf("IsDummyPlaceholderText(%q) = true, want false", nd)
		}
	}
}

func TestBuildRunCommandInstructionWithNameAndPrefix(t *testing.T) {
	cmds := []CommandInfo{
		{
			Name:        "ping",
			Alias:       "p",
			Description: "Check bot latency",
			IsPublic:    true,
		},
		{
			Name:        "eval",
			Alias:       "e",
			Description: "Execute Go code safely",
			IsPublic:    false,
		},
	}

	instruction := BuildRunCommandInstructionWithNameAndPrefix(cmds, "TestBot", "!")

	if !strings.Contains(instruction, "TestBot") {
		t.Errorf("instruction missing bot name: %s", instruction)
	}
	if !strings.Contains(instruction, "!ping (alias: !p): Check bot latency") {
		t.Errorf("instruction missing ping command with alias: %s", instruction)
	}
	if !strings.Contains(instruction, "!eval (alias: !e) [sudo-only]: Execute Go code safely") {
		t.Errorf("instruction missing eval command with sudo flag: %s", instruction)
	}
	if !strings.Contains(instruction, "RUN_COMMAND: !<command_name>") {
		t.Errorf("instruction missing RUN_COMMAND template with prefix: %s", instruction)
	}
}

func TestExtractMetaAiText(t *testing.T) {
	// 1. Plain conversation
	msgConv := &waE2E.Message{
		Conversation: proto.String("Hello from Meta AI!"),
	}
	if got := ExtractMetaAiText(msgConv); got != "Hello from Meta AI!" {
		t.Errorf("ExtractMetaAiText(Conversation) = %q, want %q", got, "Hello from Meta AI!")
	}

	// 2. Extended text message
	msgExt := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("Extended answer text"),
		},
	}
	if got := ExtractMetaAiText(msgExt); got != "Extended answer text" {
		t.Errorf("ExtractMetaAiText(ExtendedTextMessage) = %q, want %q", got, "Extended answer text")
	}

	// 3. Protocol message wrapping edited text
	msgProto := &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			EditedMessage: &waE2E.Message{
				Conversation: proto.String("Edited answer"),
			},
		},
	}
	if got := ExtractMetaAiText(msgProto); got != "Edited answer" {
		t.Errorf("ExtractMetaAiText(ProtocolMessage) = %q, want %q", got, "Edited answer")
	}

	// 4. Nil message
	if got := ExtractMetaAiText(nil); got != "" {
		t.Errorf("ExtractMetaAiText(nil) = %q, want empty string", got)
	}
}

func TestParseUnifiedMediaState(t *testing.T) {
	// 1. JSON in Conversation containing media URL and imagine status
	jsonPayload := `{
		"response_id": "resp_12345",
		"sections": [
			{
				"view_model": {
					"primitive": {
						"__typename": "MediaSection",
						"imagine_type": "IMAGINE",
						"media": {
							"url": "https://example.com/generated_art.jpg",
							"mime_type": "image/jpeg"
						},
						"status": {
							"status": "COMPLETED",
							"update_text": "Here is your generated image"
						},
						"text": "A cybernetic dragon"
					}
				}
			}
		]
	}`

	msg := &waE2E.Message{
		Conversation: proto.String(jsonPayload),
	}

	mediaURL, mimeType, text, imagineType, status := parseUnifiedMediaState(msg)

	if mediaURL != "https://example.com/generated_art.jpg" {
		t.Errorf("parseUnifiedMediaState mediaURL = %q, want https://example.com/generated_art.jpg", mediaURL)
	}
	if mimeType != "image/jpeg" {
		t.Errorf("parseUnifiedMediaState mimeType = %q, want image/jpeg", mimeType)
	}
	if text != "A cybernetic dragon" {
		t.Errorf("parseUnifiedMediaState text = %q, want 'A cybernetic dragon'", text)
	}
	if imagineType != "IMAGINE" {
		t.Errorf("parseUnifiedMediaState imagineType = %q, want IMAGINE", imagineType)
	}
	if status != "COMPLETED" {
		t.Errorf("parseUnifiedMediaState status = %q, want COMPLETED", status)
	}
}

func TestParseUnifiedMediaState_RichResponse(t *testing.T) {
	jsonPayload := `{
		"response_id": "resp_rich_99",
		"sections": [
			{
				"view_model": {
					"primitive": {
						"__typename": "MediaSection",
						"imagine_type": "ANIMATE",
						"media": {
							"url": "https://example.com/video.mp4",
							"mime_type": "video/mp4"
						},
						"status": {
							"status": "GENERATING",
							"update_text": "Animating image..."
						},
						"text": "Animated loop"
					}
				}
			}
		]
	}`

	msg := &waE2E.Message{
		RichResponseMessage: &waE2E.AIRichResponseMessage{
			UnifiedResponse: &waAICommon.AIRichResponseUnifiedResponse{
				Data: []byte(jsonPayload),
			},
		},
	}

	mediaURL, mimeType, text, imagineType, status := parseUnifiedMediaState(msg)

	if mediaURL != "https://example.com/video.mp4" {
		t.Errorf("parseUnifiedMediaState mediaURL = %q, want https://example.com/video.mp4", mediaURL)
	}
	if mimeType != "video/mp4" {
		t.Errorf("parseUnifiedMediaState mimeType = %q, want video/mp4", mimeType)
	}
	if text != "Animated loop" {
		t.Errorf("parseUnifiedMediaState text = %q, want 'Animated loop'", text)
	}
	if imagineType != "ANIMATE" {
		t.Errorf("parseUnifiedMediaState imagineType = %q, want ANIMATE", imagineType)
	}
	if status != "GENERATING" {
		t.Errorf("parseUnifiedMediaState status = %q, want GENERATING", status)
	}
}

func TestBuildAiQuery_GroupWithoutMetadata(t *testing.T) {
	instruction := "[SYSTEM INSTRUCTION]\n"
	data := Data{
		ChatType: "group",
		// Empty GroupMetaData
		GroupMetaData: types.GroupInfo{},
		PushName:      "『𖥠』ємρєяσя{ 𝕱𝖆𝖙𝖍𝖊𝖗 𝖔𝖋 𝕷𝖔𝖗𝖉𝖘 }",
		Question:      "Summarize our project goals",
	}

	query := BuildAiQuery(instruction, "", data)

	if strings.Contains(query, "[GROUP CONTEXT]") {
		t.Errorf("query should not contain [GROUP CONTEXT] when group metadata is empty: %s", query)
	}
	if !strings.Contains(query, "[USER CONTEXT]") {
		t.Errorf("query must contain [USER CONTEXT]: %s", query)
	}
	if !strings.Contains(query, "User: 『𖥠』ємρєяσя{ 𝕱𝖆𝖙𝖍𝖊𝖗 𝖔𝖋 𝕷𝖔𝖗𝖉𝖘 }") {
		t.Errorf("query must contain user display name: %s", query)
	}
	if !strings.Contains(query, "[CURRENT MESSAGE]") {
		t.Errorf("query must contain [CURRENT MESSAGE]: %s", query)
	}
	if !strings.Contains(query, "Message: Summarize our project goals") {
		t.Errorf("query must contain message text: %s", query)
	}
}

func TestCleanAiResponseText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Clean plain response without scaffolding",
			input:    "Life is a biological process characterized by metabolism, growth, and reproduction.",
			expected: "Life is a biological process characterized by metabolism, growth, and reproduction.",
		},
		{
			name: "Echoed SYSTEM CONTEXT block",
			input: `[SYSTEM CONTEXT:
You are WhatsRook, a helpful, intelligent, and capable AI assistant on WhatsApp.
Available bot commands:
- !ping: ping
]
Life is a characteristic that distinguishes physical entities.`,
			expected: "Life is a characteristic that distinguishes physical entities.",
		},
		{
			name: "Echoed CURRENT MESSAGE block",
			input: `[CURRENT MESSAGE]
From: Thruqe
Message: what is life
[/CURRENT MESSAGE]
Life is a fundamental feature of living organisms.`,
			expected: "Life is a fundamental feature of living organisms.",
		},
		{
			name: "Echoed GROUP CONTEXT block",
			input: `[GROUP CONTEXT]
Group Name: Test Group
[/GROUP CONTEXT]
Hello everyone in Test Group!`,
			expected: "Hello everyone in Test Group!",
		},
		{
			name: "Echoed USER CONTEXT block",
			input: `[USER CONTEXT]
User: Thruqe
Status: Owner/Sudo
[/USER CONTEXT]
Greetings Thruqe, how can I assist you?`,
			expected: "Greetings Thruqe, how can I assist you?",
		},
		{
			name: "Echoed REPLYING TO A MESSAGE context block",
			input: `[REPLYING TO A MESSAGE — EXTRACTED CONTEXT]
From: Alice
Message Content: What is the weather?
[/REPLYING TO A MESSAGE — EXTRACTED CONTEXT]
The weather is sunny.`,
			expected: "The weather is sunny.",
		},
		{
			name: "Echoed GLOBAL BOT PERSONALITY block",
			input: `[GLOBAL BOT PERSONALITY & RELATIONSHIP BEHAVIOR INSTRUCTION]
Always be polite.

The answer is 42.`,
			expected: "The answer is 42.",
		},
		{
			name:     "Stray tags cleanup",
			input:    "[/CURRENT MESSAGE] Here is the final answer. [/GROUP CONTEXT]",
			expected: "Here is the final answer.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanAiResponseText(tc.input)
			if got != tc.expected {
				t.Errorf("CleanAiResponseText() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestExtractMetaAiText_StrippingScaffolding(t *testing.T) {
	rawWithSystemPrompt := `[SYSTEM CONTEXT:
You are WhatsRook
]
Here is your answer.`
	msg := &waE2E.Message{
		Conversation: proto.String(rawWithSystemPrompt),
	}

	extracted := ExtractMetaAiText(msg)
	if extracted != "Here is your answer." {
		t.Errorf("ExtractMetaAiText did not strip system prompt: got %q, want %q", extracted, "Here is your answer.")
	}
}

func TestIsBotTaggedOrReplied(t *testing.T) {
	botJID := types.NewJID("123456", types.DefaultUserServer)
	client := &whatsmeow.Client{
		Store: &store.Device{
			ID: &botJID,
		},
	}

	// 1. In DM (non-group chat), isBotTaggedOrReplied should return true
	dmChat := types.NewJID("987654", types.DefaultUserServer)
	dmEvt := &events.Message{
		Info: types.MessageInfo{
			Chat: dmChat,
		},
		Message: &waE2E.Message{
			Conversation: proto.String("hello"),
		},
	}
	dmCtx := &dispatch.Context{
		Client: client,
		Evt:    dmEvt,
		Chat:   dmChat,
	}
	if !isBotTaggedOrReplied(dmCtx, "hello") {
		t.Errorf("expected isBotTaggedOrReplied to return true for DM chat")
	}

	// 2. In Group chat with bot name mention, should return true
	groupChat := types.NewJID("12345-67890", types.GroupServer)
	groupEvt := &events.Message{
		Info: types.MessageInfo{
			Chat: groupChat,
		},
		Message: &waE2E.Message{
			Conversation: proto.String("hey rook what is the time"),
		},
	}
	groupCtx := &dispatch.Context{
		Client: client,
		Evt:    groupEvt,
		Chat:   groupChat,
	}
	if !isBotTaggedOrReplied(groupCtx, "hey rook what is the time") {
		t.Errorf("expected isBotTaggedOrReplied to return true when 'rook' is mentioned")
	}

	// 3. In Group chat without mention/tag, should return false
	if isBotTaggedOrReplied(groupCtx, "regular group chat message") {
		t.Errorf("expected isBotTaggedOrReplied to return false without mention/tag")
	}
}

func TestHandleAutoAIIntercept_Filtering(t *testing.T) {
	// 1. Nil context or client should return false
	if HandleAutoAIIntercept(nil, "hello") {
		t.Errorf("expected nil context to return false")
	}

	botJID := types.NewJID("123456", types.DefaultUserServer)
	client := &whatsmeow.Client{
		Store: &store.Device{
			ID: &botJID,
		},
	}

	// 2. Outgoing message (IsFromMe=true) in a chat with someone else should return false
	otherChat := types.NewJID("999999", types.DefaultUserServer)
	evtFromMe := &events.Message{
		Info: types.MessageInfo{
			Chat:     otherChat,
			IsFromMe: true,
		},
		Message: &waE2E.Message{
			Conversation: proto.String("hello"),
		},
	}
	ctxFromMe := &dispatch.Context{
		Client: client,
		Evt:    evtFromMe,
		Chat:   otherChat,
	}
	if HandleAutoAIIntercept(ctxFromMe, "hello") {
		t.Errorf("expected IsFromMe to other chats to return false")
	}
}
