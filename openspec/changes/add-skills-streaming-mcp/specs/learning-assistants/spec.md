## ADDED Requirements

### Requirement: Class-scoped assistants and bindings
The system SHALL let teachers maintain named assistants, role prompts, enabled state, and bindings to same-class skills and materials. Students SHALL only select enabled assistants in their own class.

#### Scenario: Cross-class resource binding
- **WHEN** a teacher binds another class's material or skill
- **THEN** the system returns the same 404 response as a missing resource

### Requirement: Uploaded skills and deterministic routing
The system SHALL parse and preserve UTF-8 skill files, reject invalid or duplicate metadata, and route at most one bound enabled skill by lexical score, breaking ties by the lowest ID.

#### Scenario: Skill disabled during an existing conversation
- **WHEN** a teacher disables the matched skill
- **THEN** the next question in the existing conversation no longer selects it

### Requirement: Owned conversations with streamed evidence
The system SHALL lock an assistant at conversation creation, authorize every read by owner and class, retrieve at most four bound chunks, include at most six historical messages of 500 characters, and emit meta before incremental delta and done.

#### Scenario: Retrieval misses
- **WHEN** no bound material matches the question
- **THEN** the model still answers using the assistant role, citations are empty, and numbered citations are removed

#### Scenario: Prompt and skill inspection
- **WHEN** the owner opens request details
- **THEN** actual model request JSON and response JSON or SSE frames are shown without authentication headers
