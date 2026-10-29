package relay

type ResponseTaskBinding struct {
	ChannelID, GroupDefinitionID                                     int64
	CredentialIdentity, AccountIdentity, Model, Group, UpstreamModel string
	HasImages, HasTools, HasReasoning                                bool
	ConversationID, ConversationNonce                                string
}
type BackgroundResponseSnapshot struct {
	Binding     ResponseTaskBinding `json:"binding"`
	PriceData   PriceData           `json:"price_data"`
	InputTokens int64               `json:"input_tokens"`
	CreatedAt   int64               `json:"created_at"`
}
