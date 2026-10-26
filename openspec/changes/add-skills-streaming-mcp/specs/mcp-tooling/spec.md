## ADDED Requirements

### Requirement: Independent authenticated MCP service
The system SHALL build a separate mcp-server service in Compose, expose health and authenticated JSON-RPC initialize, tools/list and tools/call, and provide at least two tools with names, descriptions and input schemas.

#### Scenario: Invalid tool input
- **WHEN** a call supplies parameters violating the discovered schema
- **THEN** the host rejects it before execution and the server also rejects invalid direct requests

### Requirement: Atomic connection and private credentials
The host SHALL perform initialize then tools/list, replace the discovery snapshot on success, clear it on disconnect or connection failure, scope configuration by class, and return only has_token rather than stored credentials.

#### Scenario: Repeated connection
- **WHEN** a teacher connects the same server twice
- **THEN** the discovered tool count does not double

### Requirement: Model decisions with bounded host execution
The host SHALL restrict tools to the selected skill, pass discovered declarations to the model, reauthorize and validate every model proposal, filter tool data, and append tool-role results before requesting the final streamed answer.

#### Scenario: Tool budget exhausted
- **WHEN** the round or conversation call limit is reached
- **THEN** tool execution stops and the model answers using information already available

#### Scenario: Tool service fails after discovery
- **WHEN** a model selects a previously discovered tool while its server is unreachable
- **THEN** an error tool message and append-only audit record are produced while the answer path continues

### Requirement: Scoped observable calls
The system SHALL emit tool events before answer text and append a record containing tool reference, outcome, actor, conversation, round and timestamp for every attributable call attempt.

#### Scenario: Cross-class audit access
- **WHEN** another class's teacher requests the audit list
- **THEN** no records from the first class are returned
