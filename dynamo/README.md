# DynamoDB Schema and Access Patterns

This package manages interactions with Amazon DynamoDB, serving as the persistence layer for the event registration service. It employs a single-table design pattern to store `Event` and `Registration` entities.

## Table Structure

All entities are stored in a single DynamoDB table, whose name is configured at runtime.

### Primary Key (PK/SK)

The primary key uses a composite key structure (`PK` and `SK`) to distinguish between different entity types and enable efficient access patterns.

-   **Partition Key (PK):**
    -   For `Event` entities: `EVENT#<EventID>`
    -   For `Registration` entities: `EVENT#<EventID>` (This links registrations directly to their respective events)

-   **Sort Key (SK):**
    -   For `Event` entities: `EVENT#<EventID>`
    -   For `Registration` entities: `REGISTRATION#<RegistrationID>`

### Global Secondary Index (GSI1)

A Global Secondary Index named `GSI1` is used to facilitate querying events based on their type and start time.

-   **GSI1 Partition Key (GSI1PK):** `EVENT` (a static value for all event entities)
-   **GSI1 Sort Key (GSI1SK):** `EVENT#<StartTime>#<EventID>` (allows sorting events by their start time)

## Entity Schemas

### Event Entity

Represents an event in the system.

| Attribute             | Type          | Description                                     | Example Value                                   |
| :-------------------- | :------------ | :---------------------------------------------- | :---------------------------------------------- |
| `PK`                  | String        | Partition Key: `EVENT#<EventID>`                | `EVENT#a1b2c3d4-e5f6-7890-1234-567890abcdef`    |
| `SK`                  | String        | Sort Key: `EVENT#<EventID>`                     | `EVENT#a1b2c3d4-e5f6-7890-1234-567890abcdef`    |
| `GSI1PK`              | String        | GSI1 Partition Key: `EVENT`                     | `EVENT`                                         |
| `GSI1SK`              | String        | GSI1 Sort Key: `EVENT#<StartTime>#<EventID>`    | `EVENT#2025-08-18T10:00:00Z#a1b2c3d4-e5f6-7890-1234-567890abcdef` |
| `ID`                  | UUID          | Unique identifier for the event                 | `a1b2c3d4-e5f6-7890-1234-567890abcdef`          |
| `Version`             | Number        | Optimistic locking version                      | `1`                                             |
| `Name`                | String        | Name of the event                               | `Summer Archery Tournament`                     |
| `EventLocation`       | Map           | Details of the event's location                 | `{ "Address": "123 Main St", "City": "Anytown" }` |
| `StartTime`           | Timestamp     | Event start time (ISO 8601)                     | `2025-08-18T10:00:00Z`                          |
| `EndTime`             | Timestamp     | Event end time (ISO 8601)                       | `2025-08-18T18:00:00Z`                          |
| `RegistrationCloseTime` | Timestamp     | Time when registration closes (ISO 8601)        | `2025-08-17T23:59:59Z`                          |
| `RegistrationTypes`   | List of Strings | Allowed registration types (e.g., `BY_INDIVIDUAL`, `BY_TEAM`) | `["BY_INDIVIDUAL", "BY_TEAM"]`                  |
| `AllowedTeamSizeRange`| Map           | Min and Max team size for team registrations    | `{ "Min": 2, "Max": 5 }`                      |
| `NumTeams`            | Number        | Number of teams registered for the event        | `5`                                             |
| `NumRosteredPlayers`  | Number        | Number of players rostered across all teams     | `20`                                            |
| `NumTotalPlayers`     | Number        | Total number of players registered for the event| `25`                                            |

### Registration Entity

Represents a registration for an event. This entity is polymorphic, storing either individual or team-specific attributes.

| Attribute             | Type          | Description                                     | Example Value                                   |
| :-------------------- | :------------ | :---------------------------------------------- | :---------------------------------------------- |
| `PK`                  | String        | Partition Key: `EVENT#<EventID>`                | `EVENT#a1b2c3d4-e5f6-7890-1234-567890abcdef`    |
| `SK`                  | String        | Sort Key: `REGISTRATION#<RegistrationID>`       | `REGISTRATION#fedcba98-7654-3210-fedc-ba9876543210` |
| `Type`                | String        | Type of registration (`BY_INDIVIDUAL` or `BY_TEAM`) | `BY_INDIVIDUAL`                                 |
| `ID`                  | UUID          | Unique identifier for the registration          | `fedcba98-7654-3210-fedc-ba9876543210`          |
| `Version`             | Number        | Optimistic locking version                      | `1`                                             |
| `EventID`             | UUID          | ID of the event this registration is for        | `a1b2c3d4-e5f6-7890-1234-567890abcdef`          |
| `RegisteredAt`        | Timestamp     | Time of registration (ISO 8601)                 | `2025-08-18T11:30:00Z`                          |
| `HomeCity`            | String        | Registrant's home city                          | `Anytown`                                       |
| `Paid`                | Boolean       | Whether the registration has been paid          | `true`                                          |
| `Email`               | String        | (Individual) Registrant's email                 | `john.doe@example.com`                          |
| `PlayerInfo`          | Map           | (Individual) Player details                     | `{ "Name": "John Doe", "Age": 30 }`             |
| `Experience`          | String        | (Individual) Experience level                   | `BEGINNER`                                      |
| `TeamName`            | String        | (Team) Name of the team                         | `Archery Avengers`                              |
| `CaptainEmail`        | String        | (Team) Email of the team captain                | `jane.doe@example.com`                          |
| `Players`             | List of Maps  | (Team) List of player details                   | `[{ "Name": "Jane Doe" }, { "Name": "Peter Pan" }]` |

## Access Patterns

The following are the primary access patterns implemented in this package:

### Event Access Patterns

-   **Get Event by ID:**
    -   **Operation:** `GetItem`
    -   **Keys:** `PK = EVENT#<EventID>`, `SK = EVENT#<EventID>`
    -   **Purpose:** Retrieve a single event by its unique identifier.

-   **Create Event:**
    -   **Operation:** `PutItem` with conditional check
    -   **Condition:** Ensures the event does not already exist (PK/SK combination is new) and the version is 1.
    -   **Purpose:** Persist a new event.

-   **Update Event:**
    -   **Operation:** `PutItem` with conditional check
    -   **Condition:** Ensures the event exists and the version matches for optimistic locking.
    -   **Purpose:** Modify an existing event.

-   **List Events (Paginated):**
    -   **Operation:** `Query` on `GSI1`
    -   **Keys:** `GSI1PK = EVENT`, `GSI1SK` begins with `EVENT` (allowing for time-based sorting)
    -   **Purpose:** Retrieve a list of events, typically for display or browsing, with support for pagination.

### Registration Access Patterns

-   **Get Registration by Event ID and Registration ID:**
    -   **Operation:** `GetItem`
    -   **Keys:** `PK = EVENT#<EventID>`, `SK = REGISTRATION#<RegistrationID>`
    -   **Purpose:** Retrieve a specific registration for a given event.

-   **Create Registration (Transactional):**
    -   **Operation:** `TransactWriteItems` (Put Registration and Update Event)
    -   **Conditions:**
        -   Registration: Ensures the registration does not already exist and the version is 1.
        -   Event: Ensures the event exists and its version matches for optimistic locking (to increment event version upon new registration).
    -   **Purpose:** Atomically create a new registration and update the associated event's version.

-   **List All Registrations for an Event (Paginated):**
    -   **Operation:** `Query` on the base table
    -   **Keys:** `PK = EVENT#<EventID>`, `SK` begins with `REGISTRATION`
    -   **Purpose:** Retrieve all registrations associated with a specific event, with support for pagination.
