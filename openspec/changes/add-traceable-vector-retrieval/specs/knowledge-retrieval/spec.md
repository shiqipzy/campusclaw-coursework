## ADDED Requirements

### Requirement: Authorized class-scoped retrieval

The system SHALL allow authenticated teachers and students to search only material chunks belonging to the class in their server-side session. It SHALL ignore any class identifier supplied in the request. Both the chunk query and the material lookup SHALL enforce that session class.

#### Scenario: Same-class phrase

- **WHEN** a signed-in student searches for a phrase present in their class material
- **THEN** the response is HTTP 200 and includes the matching chunk from that class

#### Scenario: Forged class identifier

- **WHEN** a signed-in A-class user searches for a phrase present only in B-class material and supplies B's `class_id` in the query
- **THEN** the response is HTTP 200 with an empty `hits` array and does not disclose B-class material metadata

#### Scenario: Anonymous request

- **WHEN** a user without a valid session calls `GET /api/search`
- **THEN** the response is HTTP 401

### Requirement: Searchable source chunks

The system SHALL split each uploaded material into Unicode-safe, overlapping chunks, save their original character ranges, and make previously stored materials searchable without changing their source text.

#### Scenario: Teacher uploads a material

- **WHEN** a teacher uploads a valid text or Markdown material
- **THEN** the material, source text, and its chunks are committed together and a student in that class can search its phrases

#### Scenario: Existing material is indexed

- **WHEN** the upgraded service starts with an existing material that has no chunks
- **THEN** it creates chunks from the saved source text and repeated startup does not duplicate them

#### Scenario: Exact source range

- **WHEN** a result reports `char_start` and `char_end`
- **THEN** the Unicode character slice of the stored source text at `[char_start,char_end)` equals that chunk's text

### Requirement: Traceable search results

The system SHALL return a material title, material ID, chunk ID, one-based chunk index, original character range, excerpt taken from the stored chunk, and a protected material URL for every hit.

#### Scenario: Open a hit source

- **WHEN** a user selects a hit from their own class
- **THEN** the protected material URL opens the corresponding original material

#### Scenario: Cross-class source URL

- **WHEN** a user opens the material URL for another class, or an unknown material ID
- **THEN** both requests return HTTP 404

### Requirement: No-evidence and input behavior

The system SHALL return no fabricated result when no chunk contains the searched phrase. It SHALL reject queries shorter than 2 or longer than 200 Unicode characters.

#### Scenario: No matching source

- **WHEN** the class contains no chunk with the searched phrase
- **THEN** the response is HTTP 200 with `hits: []` and `message: "资料中未找到相关内容"`

#### Scenario: Invalid query length

- **WHEN** `q` has fewer than 2 or more than 200 Unicode characters after trimming outer whitespace
- **THEN** the response is HTTP 400
