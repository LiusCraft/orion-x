// Package main contains the manager HTTP server.
//
// @title Orion X Manager API
// @version 1.0
// @description Management API for users, voicebots, providers, models, MCP servers, memory, and knowledge bases.
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter a JWT as `Bearer <token>`.
//
//nolint:unused // Swagger operation declarations below are consumed by swag at generation time.
package main

// errorResponse is the common error response returned by manager endpoints.
type errorResponse struct {
	Error string `json:"error"`
}

// swaggerLogin documents the login endpoint.
// @Summary Log in
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body object true "Username and password"
// @Success 200 {object} object
// @Failure 400,401 {object} errorResponse
// @Router /api/auth/login [post]
func swaggerLogin() {}

// swaggerChangePassword documents password updates.
// @Summary Change password
// @Tags Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Current and new password"
// @Success 200 {object} object
// @Failure 400,401,404 {object} errorResponse
// @Router /api/auth/change-password [post]
func swaggerChangePassword() {}

// swaggerBindEmail documents email binding.
// @Summary Bind email
// @Tags Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Email address"
// @Success 200 {object} object
// @Failure 400,401 {object} errorResponse
// @Router /api/auth/bind-email [post]
func swaggerBindEmail() {}

// swaggerProfile documents the current user profile.
// @Summary Get current profile
// @Tags Authentication
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object
// @Failure 401,404 {object} errorResponse
// @Router /api/auth/profile [get]
func swaggerProfile() {}

// swaggerVoicebots documents voicebot collection operations.
// @Summary List voicebots
// @Tags Voicebots
// @Produce json
// @Security BearerAuth
// @Success 200 {array} object
// @Failure 401 {object} errorResponse
// @Router /api/voicebots [get]
func swaggerListVoicebots() {}

// swaggerCreateVoicebot creates a voicebot.
// @Summary Create voicebot
// @Tags Voicebots
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Voicebot fields"
// @Success 201 {object} object
// @Failure 400,401 {object} errorResponse
// @Router /api/voicebots [post]
func swaggerCreateVoicebot() {}

// swaggerVoicebot documents operations on one voicebot.
// @Summary Get voicebot
// @Tags Voicebots
// @Produce json
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/voicebots/{id} [get]
func swaggerGetVoicebot() {}

// swaggerUpdateVoicebot updates a voicebot.
// @Summary Update voicebot
// @Tags Voicebots
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Param request body object true "Voicebot fields"
// @Success 200 {object} object
// @Failure 400,401,403,404 {object} errorResponse
// @Router /api/voicebots/{id} [put]
func swaggerUpdateVoicebot() {}

// swaggerDeleteVoicebot deletes a voicebot.
// @Summary Delete voicebot
// @Tags Voicebots
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Success 204
// @Failure 401,403,404 {object} errorResponse
// @Router /api/voicebots/{id} [delete]
func swaggerDeleteVoicebot() {}

// swaggerListDevices lists a voicebot's devices.
// @Summary List devices
// @Tags Devices
// @Produce json
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Success 200 {array} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/voicebots/{id}/devices [get]
func swaggerListDevices() {}

// swaggerCreateDevice creates a device for a voicebot.
// @Summary Create device
// @Tags Devices
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Param request body object true "Device fields"
// @Success 201 {object} object
// @Failure 400,401,403,404,409 {object} errorResponse
// @Router /api/voicebots/{id}/devices [post]
func swaggerCreateDevice() {}

// swaggerDeleteDevice deletes a device.
// @Summary Delete device
// @Tags Devices
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Param did path string true "Device ID"
// @Success 204
// @Failure 401,403,404 {object} errorResponse
// @Router /api/voicebots/{id}/devices/{did} [delete]
func swaggerDeleteDevice() {}

// swaggerSetTelegramChannel configures a Telegram channel.
// @Summary Set Telegram channel
// @Tags Devices
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Param did path string true "Device ID"
// @Param request body object true "Telegram bot token"
// @Success 200 {object} object
// @Failure 400,401,403,404 {object} errorResponse
// @Router /api/voicebots/{id}/devices/{did}/channels/telegram [put]
func swaggerSetTelegramChannel() {}

// swaggerDeleteTelegramChannel removes a Telegram channel.
// @Summary Remove Telegram channel
// @Tags Devices
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Param did path string true "Device ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/voicebots/{id}/devices/{did}/channels/telegram [delete]
func swaggerDeleteTelegramChannel() {}

// swaggerProviders documents provider collection operations.
// @Summary List providers
// @Tags Providers
// @Produce json
// @Security BearerAuth
// @Success 200 {array} object
// @Failure 401 {object} errorResponse
// @Router /api/providers [get]
func swaggerListProviders() {}

// swaggerCreateProvider creates a provider.
// @Summary Create provider
// @Tags Providers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Provider fields"
// @Success 201 {object} object
// @Failure 400,401 {object} errorResponse
// @Router /api/providers [post]
func swaggerCreateProvider() {}

// swaggerProviderSlugs lists known provider slugs.
// @Summary List provider slugs
// @Tags Providers
// @Produce json
// @Security BearerAuth
// @Success 200 {array} string
// @Failure 401 {object} errorResponse
// @Router /api/providers/slugs [get]
func swaggerProviderSlugs() {}

// swaggerGetProvider gets a provider.
// @Summary Get provider
// @Tags Providers
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider ID"
// @Success 200 {object} object
// @Failure 401,404 {object} errorResponse
// @Router /api/providers/{id} [get]
func swaggerGetProvider() {}

// swaggerUpdateProvider updates a provider.
// @Summary Update provider
// @Tags Providers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider ID"
// @Param request body object true "Provider fields"
// @Success 200 {object} object
// @Failure 400,401,403,404 {object} errorResponse
// @Router /api/providers/{id} [put]
func swaggerUpdateProvider() {}

// swaggerDeleteProvider deletes a provider.
// @Summary Delete provider
// @Tags Providers
// @Security BearerAuth
// @Param id path string true "Provider ID"
// @Success 204
// @Failure 401,403,404 {object} errorResponse
// @Router /api/providers/{id} [delete]
func swaggerDeleteProvider() {}

// swaggerModels documents model collection operations.
// @Summary List models
// @Tags Models
// @Produce json
// @Security BearerAuth
// @Success 200 {array} object
// @Failure 401 {object} errorResponse
// @Router /api/models [get]
func swaggerListModels() {}

// swaggerCreateModel creates a model.
// @Summary Create model
// @Tags Models
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Model fields"
// @Success 201 {object} object
// @Failure 400,401 {object} errorResponse
// @Router /api/models [post]
func swaggerCreateModel() {}

// swaggerModelTypes lists supported model types.
// @Summary List model types
// @Tags Models
// @Produce json
// @Security BearerAuth
// @Success 200 {array} string
// @Failure 401 {object} errorResponse
// @Router /api/models/types [get]
func swaggerModelTypes() {}

// swaggerGetModel gets a model.
// @Summary Get model
// @Tags Models
// @Produce json
// @Security BearerAuth
// @Param id path string true "Model ID"
// @Success 200 {object} object
// @Failure 401,404 {object} errorResponse
// @Router /api/models/{id} [get]
func swaggerGetModel() {}

// swaggerUpdateModel updates a model.
// @Summary Update model
// @Tags Models
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Model ID"
// @Param request body object true "Model fields"
// @Success 200 {object} object
// @Failure 400,401,403,404 {object} errorResponse
// @Router /api/models/{id} [put]
func swaggerUpdateModel() {}

// swaggerDeleteModel deletes a model.
// @Summary Delete model
// @Tags Models
// @Security BearerAuth
// @Param id path string true "Model ID"
// @Success 204
// @Failure 401,403,404 {object} errorResponse
// @Router /api/models/{id} [delete]
func swaggerDeleteModel() {}

// swaggerListSystemVoices lists system voices.
// @Summary List system voices
// @Tags Voices
// @Produce json
// @Security BearerAuth
// @Success 200 {array} object
// @Failure 401 {object} errorResponse
// @Router /api/voices/system [get]
func swaggerListSystemVoices() {}

// swaggerListVoices lists a model's voices.
// @Summary List model voices
// @Tags Voices
// @Produce json
// @Security BearerAuth
// @Param id path string true "Model ID"
// @Success 200 {array} object
// @Failure 401,404 {object} errorResponse
// @Router /api/models/{id}/voices [get]
func swaggerListVoices() {}

// swaggerCreateVoice creates a voice.
// @Summary Create voice
// @Tags Voices
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Model ID"
// @Param request body object true "Voice fields"
// @Success 201 {object} object
// @Failure 400,401,404 {object} errorResponse
// @Router /api/models/{id}/voices [post]
func swaggerCreateVoice() {}

// swaggerCloneVoice clones a voice.
// @Summary Clone voice
// @Tags Voices
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Model ID"
// @Param request body object true "Source voice"
// @Success 201 {object} object
// @Failure 400,401,404 {object} errorResponse
// @Router /api/models/{id}/voices/clone [post]
func swaggerCloneVoice() {}

// swaggerGetVoice gets a voice.
// @Summary Get voice
// @Tags Voices
// @Produce json
// @Security BearerAuth
// @Param id path string true "Model ID"
// @Param vid path string true "Voice ID"
// @Success 200 {object} object
// @Failure 401,404 {object} errorResponse
// @Router /api/models/{id}/voices/{vid} [get]
func swaggerGetVoice() {}

// swaggerUpdateVoice updates a voice.
// @Summary Update voice
// @Tags Voices
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Model ID"
// @Param vid path string true "Voice ID"
// @Param request body object true "Voice fields"
// @Success 200 {object} object
// @Failure 400,401,403,404 {object} errorResponse
// @Router /api/models/{id}/voices/{vid} [put]
func swaggerUpdateVoice() {}

// swaggerDeleteVoice deletes a voice.
// @Summary Delete voice
// @Tags Voices
// @Security BearerAuth
// @Param id path string true "Model ID"
// @Param vid path string true "Voice ID"
// @Success 204
// @Failure 401,403,404 {object} errorResponse
// @Router /api/models/{id}/voices/{vid} [delete]
func swaggerDeleteVoice() {}

// swaggerAvailableResources documents available resource discovery.
// @Summary List available resources
// @Tags Resources
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object
// @Failure 401 {object} errorResponse
// @Router /api/available-resources [get]
func swaggerAvailableResources() {}

// swaggerListAgents lists agents with memory.
// @Summary List memory agents
// @Tags Memory
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object
// @Failure 401 {object} errorResponse
// @Router /api/data/memory/agents [get]
func swaggerListMemoryAgents() {}

// swaggerListMemoryDevices lists an agent's devices.
// @Summary List agent devices
// @Tags Memory
// @Produce json
// @Security BearerAuth
// @Param agent_id path string true "Agent ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/data/memory/agents/{agent_id}/devices [get]
func swaggerListMemoryDevices() {}

// swaggerListMemoryEntries lists device memory entries.
// @Summary List memory entries
// @Tags Memory
// @Produce json
// @Security BearerAuth
// @Param device_id path string true "Device ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/data/memory/devices/{device_id}/entries [get]
func swaggerListMemoryEntries() {}

// swaggerDeleteMemory deletes a memory entry.
// @Summary Delete memory entry
// @Tags Memory
// @Security BearerAuth
// @Param id path string true "Memory entry ID"
// @Success 204
// @Failure 401,404 {object} errorResponse
// @Router /api/data/memory/{id} [delete]
func swaggerDeleteMemory() {}

// swaggerKnowledgeBases lists knowledge bases.
// @Summary List knowledge bases
// @Tags Knowledge
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object
// @Failure 401,503 {object} errorResponse
// @Router /api/data/knowledge/knowledge_bases [get]
func swaggerListKnowledgeBases() {}

// swaggerGetKnowledgeBase gets a knowledge base.
// @Summary Get knowledge base
// @Tags Knowledge
// @Produce json
// @Security BearerAuth
// @Param kb_id path string true "Knowledge base ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/data/knowledge/knowledge_bases/{kb_id} [get]
func swaggerGetKnowledgeBase() {}

// swaggerSearchKnowledgeBase searches a knowledge base.
// @Summary Search knowledge base
// @Tags Knowledge
// @Produce json
// @Security BearerAuth
// @Param kb_id path string true "Knowledge base ID"
// @Param q query string true "Search query"
// @Success 200 {array} object
// @Failure 400,401,403,404 {object} errorResponse
// @Router /api/data/knowledge/knowledge_bases/{kb_id}/search [get]
func swaggerSearchKnowledgeBase() {}

// swaggerDeleteKnowledgeBase deletes a knowledge base.
// @Summary Delete knowledge base
// @Tags Knowledge
// @Security BearerAuth
// @Param kb_id path string true "Knowledge base ID"
// @Success 204
// @Failure 401,403,404 {object} errorResponse
// @Router /api/data/knowledge/knowledge_bases/{kb_id} [delete]
func swaggerDeleteKnowledgeBase() {}

// swaggerListDocuments lists documents in a knowledge base.
// @Summary List knowledge base documents
// @Tags Knowledge
// @Produce json
// @Security BearerAuth
// @Param kb_id path string true "Knowledge base ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/data/knowledge/knowledge_bases/{kb_id}/documents [get]
func swaggerListDocuments() {}

// swaggerUploadDocument uploads a document.
// @Summary Upload knowledge document
// @Tags Knowledge
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param kb_id path string true "Knowledge base ID"
// @Param file formData file true "Document file"
// @Success 201 {object} object
// @Failure 400,401,403,404 {object} errorResponse
// @Router /api/data/knowledge/knowledge_bases/{kb_id}/documents [post]
func swaggerUploadDocument() {}

// swaggerIngestURL imports a document from a URL.
// @Summary Import document URL
// @Tags Knowledge
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param kb_id path string true "Knowledge base ID"
// @Param request body object true "URL to import"
// @Success 201 {object} object
// @Failure 400,401,403,404 {object} errorResponse
// @Router /api/data/knowledge/knowledge_bases/{kb_id}/documents/url [post]
func swaggerIngestURL() {}

// swaggerDeleteDocument deletes a document.
// @Summary Delete knowledge document
// @Tags Knowledge
// @Security BearerAuth
// @Param doc_id path string true "Document ID"
// @Success 204
// @Failure 401,403,404 {object} errorResponse
// @Router /api/data/knowledge/documents/{doc_id} [delete]
func swaggerDeleteDocument() {}

// swaggerDocumentStatus gets document ingestion status.
// @Summary Get document status
// @Tags Knowledge
// @Produce json
// @Security BearerAuth
// @Param doc_id path string true "Document ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/data/knowledge/documents/{doc_id}/status [get]
func swaggerDocumentStatus() {}

// swaggerRetryDocument retries document processing.
// @Summary Retry document processing
// @Tags Knowledge
// @Security BearerAuth
// @Param doc_id path string true "Document ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/data/knowledge/documents/{doc_id}/retry [post]
func swaggerRetryDocument() {}

// swaggerBoundKnowledgeBases lists a bot's bound knowledge bases.
// @Summary List bound knowledge bases
// @Tags Knowledge
// @Produce json
// @Security BearerAuth
// @Param bot_id path string true "Voicebot ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/data/knowledge/bots/{bot_id}/knowledge_bases/bound [get]
func swaggerBoundKnowledgeBases() {}

// swaggerBindKnowledgeBase binds a knowledge base to a bot.
// @Summary Bind knowledge base
// @Tags Knowledge
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param bot_id path string true "Voicebot ID"
// @Param request body object true "Knowledge base ID"
// @Success 200 {object} object
// @Failure 400,401,403,404 {object} errorResponse
// @Router /api/data/knowledge/bots/{bot_id}/knowledge_bases/bind [post]
func swaggerBindKnowledgeBase() {}

// swaggerUnbindKnowledgeBase removes a bot knowledge base binding.
// @Summary Unbind knowledge base
// @Tags Knowledge
// @Security BearerAuth
// @Param bot_id path string true "Voicebot ID"
// @Param kb_id path string true "Knowledge base ID"
// @Success 204
// @Failure 401,403,404 {object} errorResponse
// @Router /api/data/knowledge/bots/{bot_id}/knowledge_bases/{kb_id}/bind [delete]
func swaggerUnbindKnowledgeBase() {}

// swaggerBotKnowledgeBases lists a bot's available knowledge bases.
// @Summary List bot knowledge bases
// @Tags Knowledge
// @Produce json
// @Security BearerAuth
// @Param bot_id path string true "Voicebot ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/data/knowledge/bots/{bot_id}/knowledge_bases [get]
func swaggerBotKnowledgeBases() {}

// swaggerCreateKnowledgeBase creates a knowledge base for a bot.
// @Summary Create bot knowledge base
// @Tags Knowledge
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param bot_id path string true "Voicebot ID"
// @Param request body object true "Knowledge base fields"
// @Success 201 {object} object
// @Failure 400,401,403,404 {object} errorResponse
// @Router /api/data/knowledge/bots/{bot_id}/knowledge_bases [post]
func swaggerCreateKnowledgeBase() {}

// swaggerSessions lists active sessions.
// @Summary List active sessions
// @Tags Sessions
// @Produce json
// @Security BearerAuth
// @Success 200 {array} object
// @Failure 401 {object} errorResponse
// @Router /api/sessions [get]
func swaggerSessions() {}

// swaggerMarket lists MCP market entries.
// @Summary List MCP market entries
// @Tags MCP
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Success 200 {object} object
// @Failure 401 {object} errorResponse
// @Router /api/mcp/market [get]
func swaggerMCPMarket() {}

// swaggerMCPServers lists MCP servers.
// @Summary List MCP servers
// @Tags MCP
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object
// @Failure 401 {object} errorResponse
// @Router /api/mcp/servers [get]
func swaggerListMCPServers() {}

// swaggerCreateMCPServer creates an MCP server.
// @Summary Create MCP server
// @Tags MCP
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "MCP server fields"
// @Success 201 {object} object
// @Failure 400,401 {object} errorResponse
// @Router /api/mcp/servers [post]
func swaggerCreateMCPServer() {}

// swaggerTestMCP tests an MCP connection.
// @Summary Test MCP connection
// @Tags MCP
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "MCP connection fields"
// @Success 200 {object} object
// @Failure 400,401 {object} errorResponse
// @Router /api/mcp/test-connection [post]
func swaggerTestMCP() {}

// swaggerListMCPTools lists tools from a server definition.
// @Summary List MCP tools
// @Tags MCP
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "MCP connection fields"
// @Success 200 {object} object
// @Failure 400,401 {object} errorResponse
// @Router /api/mcp/list-tools [post]
func swaggerListMCPTools() {}

// swaggerCallMCPTool calls an MCP tool.
// @Summary Call MCP tool
// @Tags MCP
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Tool invocation"
// @Success 200 {object} object
// @Failure 400,401 {object} errorResponse
// @Router /api/mcp/call-tool [post]
func swaggerCallMCPTool() {}

// swaggerGetMCPServer gets an MCP server.
// @Summary Get MCP server
// @Tags MCP
// @Produce json
// @Security BearerAuth
// @Param serverID path string true "MCP server ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/mcp/servers/{serverID} [get]
func swaggerGetMCPServer() {}

// swaggerUpdateMCPServer updates an MCP server.
// @Summary Update MCP server
// @Tags MCP
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param serverID path string true "MCP server ID"
// @Param request body object true "MCP server fields"
// @Success 200 {object} object
// @Failure 400,401,403,404 {object} errorResponse
// @Router /api/mcp/servers/{serverID} [put]
func swaggerUpdateMCPServer() {}

// swaggerDeleteMCPServer deletes an MCP server.
// @Summary Delete MCP server
// @Tags MCP
// @Security BearerAuth
// @Param serverID path string true "MCP server ID"
// @Success 204
// @Failure 401,403,404 {object} errorResponse
// @Router /api/mcp/servers/{serverID} [delete]
func swaggerDeleteMCPServer() {}

// swaggerVoicebotMCPs lists MCP servers bound to a voicebot.
// @Summary List voicebot MCP bindings
// @Tags MCP
// @Produce json
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/voicebots/{id}/mcps [get]
func swaggerVoicebotMCPs() {}

// swaggerBindMCP binds an MCP server to a voicebot.
// @Summary Bind MCP server to voicebot
// @Tags MCP
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Param request body object true "MCP server ID"
// @Success 201 {object} object
// @Failure 400,401,403,404,409 {object} errorResponse
// @Router /api/voicebots/{id}/mcps [post]
func swaggerBindMCP() {}

// swaggerUnbindMCP removes an MCP binding.
// @Summary Unbind MCP server from voicebot
// @Tags MCP
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Param serverID path string true "MCP server ID"
// @Success 204
// @Failure 401,403,404 {object} errorResponse
// @Router /api/voicebots/{id}/mcps/{serverID} [delete]
func swaggerUnbindMCP() {}

// swaggerToggleMCP toggles an MCP binding.
// @Summary Toggle voicebot MCP binding
// @Tags MCP
// @Produce json
// @Security BearerAuth
// @Param id path string true "Voicebot ID"
// @Param serverID path string true "MCP server ID"
// @Success 200 {object} object
// @Failure 401,403,404 {object} errorResponse
// @Router /api/voicebots/{id}/mcps/{serverID}/toggle [patch]
func swaggerToggleMCP() {}

// swaggerLanguages lists supported languages.
// @Summary List languages
// @Tags Languages
// @Produce json
// @Security BearerAuth
// @Success 200 {array} object
// @Failure 401 {object} errorResponse
// @Router /api/languages [get]
func swaggerLanguages() {}

// swaggerLanguage gets a supported language.
// @Summary Get language
// @Tags Languages
// @Produce json
// @Security BearerAuth
// @Param code path string true "Language code"
// @Success 200 {object} object
// @Failure 401,404 {object} errorResponse
// @Router /api/languages/{code} [get]
func swaggerLanguage() {}

// swaggerInternalDeviceConfig documents the device bootstrap endpoint.
// @Summary Get device configuration
// @Tags Internal
// @Produce json
// @Param device_id query string true "Device ID"
// @Success 200 {object} object
// @Failure 400,404 {object} errorResponse
// @InternalRouter /internal/device-config [get]
func swaggerInternalDeviceConfig() {}

// swaggerInternalVoiceCreate creates a system voice.
// @Summary Create system voice
// @Tags Internal
// @Accept json
// @Produce json
// @Param request body object true "Voice fields"
// @Success 201 {object} object
// @Failure 400 {object} errorResponse
// @InternalRouter /internal/voices [post]
func swaggerInternalVoiceCreate() {}

// swaggerInternalVoiceUpdate updates a system voice.
// @Summary Update system voice
// @Tags Internal
// @Accept json
// @Produce json
// @Param id path string true "Voice ID"
// @Param request body object true "Voice fields"
// @Success 200 {object} object
// @Failure 400,404 {object} errorResponse
// @InternalRouter /internal/voices/{id} [patch]
func swaggerInternalVoiceUpdate() {}

// swaggerInternalMemory documents per-device memory operations.
// @Summary Get device memory
// @Tags Internal
// @Produce json
// @Param device_id path string true "Device ID"
// @Success 200 {object} object
// @Failure 404 {object} errorResponse
// @InternalRouter /internal/devices/{device_id}/memory [get]
func swaggerInternalGetMemory() {}

// swaggerInternalPutMemory replaces device memory.
// @Summary Replace device memory
// @Tags Internal
// @Accept json
// @Param device_id path string true "Device ID"
// @Param request body object true "Memory entries"
// @Success 204
// @Failure 400,404 {object} errorResponse
// @InternalRouter /internal/devices/{device_id}/memory [put]
func swaggerInternalPutMemory() {}

// swaggerInternalTurns documents turn storage.
// @Summary Create conversation turn
// @Tags Internal
// @Accept json
// @Produce json
// @Param device_id path string true "Device ID"
// @Param request body object true "Conversation turn"
// @Success 201 {object} object
// @Failure 400 {object} errorResponse
// @InternalRouter /internal/devices/{device_id}/turns [post]
func swaggerInternalCreateTurn() {}

// swaggerInternalSearchTurns searches device turns.
// @Summary Search conversation turns
// @Tags Internal
// @Produce json
// @Param device_id path string true "Device ID"
// @Param q query string false "Search query"
// @Param limit query int false "Maximum results"
// @Success 200 {object} object
// @Failure 400 {object} errorResponse
// @InternalRouter /internal/devices/{device_id}/turns [get]
func swaggerInternalSearchTurns() {}

// swaggerInternalSessionMessages gets stored messages for a session.
// @Summary Get session messages
// @Tags Internal
// @Produce json
// @Param device_id path string true "Device ID"
// @Param session_id path string true "Session ID"
// @Success 200 {object} object
// @Failure 404 {object} errorResponse
// @InternalRouter /internal/devices/{device_id}/sessions/{session_id} [get]
func swaggerInternalSessionMessages() {}

// swaggerInternalKnowledgeSearch searches knowledge visible to a device.
// @Summary Search device knowledge
// @Tags Internal
// @Produce json
// @Param device_id query string true "Device ID"
// @Param q query string true "Search query"
// @Param top_k query int false "Maximum results"
// @Success 200 {array} object
// @Failure 400,404 {object} errorResponse
// @InternalRouter /internal/knowledge/search [get]
func swaggerInternalKnowledgeSearch() {}

// swaggerInternalTelegramBots lists devices configured with Telegram bots.
// @Summary List Telegram bot devices
// @Tags Internal
// @Produce json
// @Success 200 {array} object
// @InternalRouter /internal/devices/tg-bots [get]
func swaggerInternalTelegramBots() {}
