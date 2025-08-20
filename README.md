# Event Registration Service

This repository contains the backend service for event registration, built using Go and AWS SAM (Serverless Application Model). It handles event creation, registration, and related functionalities.

## Code Structure

The project is organized into the following main directories:

-   `api/`: Contains the API definitions, handlers, and OpenAPI specifications. This is where the HTTP endpoints are defined and implemented.
-   `cmd/`: Holds the main application entry point.
-   `dynamo/`: Manages interactions with Amazon DynamoDB, including data models and database operations for events and registrations.
-   `events/`: Defines core data structures and business logic related to events.
-   `registration/`: Defines core data structures and business logic related to registrations.
-   `spec/`: Contains the OpenAPI specification (`api.yaml`) for the service.
-   `ptr/`: Utility functions for pointers.
-   `slices/`: Utility functions for slices.

## Running Locally

To run the project locally, ensure you have Docker, Docker Compose, Go, and AWS SAM CLI installed.

1.  **Build the project:**
    ```bash
    make build
    ```
    This command generates necessary Go code and builds the SAM application.

2.  **Start local services and API:**
    ```bash
    make local
    ```
    This command will:
    -   Build the project (if not already built).
    -   Start local DynamoDB and other necessary services using `docker-compose`.
    -   Start the SAM local API gateway, making the API accessible on your local machine.

    You can then interact with the API endpoints, typically available at `http://localhost:3000` (or a similar port reported by SAM CLI).

    testing
