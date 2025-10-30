# Dexter Discord Interface - Development Tasks

This document outlines the development tasks derived from the analysis in `REPORT.md`. The tasks are prioritized to address the most critical issues first, followed by major architectural improvements, performance enhancements, and code quality refinements.

## Task List

### Priority 1: Critical Fixes

-   [ ] **`cache` Module:**
    -   [ ] Modify `BulkInsertMessages` to use a Redis transaction, ensuring that the deletion of old messages and insertion of new ones is an atomic operation. This will prevent potential data loss if the process is interrupted.

### Priority 2: Architectural Refactoring

-   [ ] **`events` Module:**
    -   [ ] Refactor the `Handler` struct, which has become a "god object". Break it down into smaller, more focused handlers for different event types to improve modularity and testability.
    -   [ ] Decompose the `processLLMResponse` function in `llm.go` into smaller, more manageable functions to simplify the logic of LLM interactions.
    -   [ ] Break down the `formatConnectionMessage` function in `voice.go` to improve readability and maintainability.
    -   [ ] Simplify the state management for user interactions, possibly by implementing a more formal state machine pattern.

-   [ ] **`app` Module:**
    -   [ ] Refactor the `NewApp` function. Introduce a dependency injection container or use a builder pattern to decouple component initialization.
    -   [ ] Establish and enforce a consistent error handling strategy across the entire application.

### Priority 3: Performance and Reliability

-   [ ] **`llm` Module:**
    -   [ ] Replace the brittle string-splitting logic for parsing LLM responses with a more robust method, such as expecting a JSON object.
    -   [ ] Implement a buffering mechanism in `processStream` to batch updates to Discord messages, avoiding rate-limiting issues caused by frequent edits.

-   [ ] **`cache` Module:**
    -   [ ] Optimize the `cleanKeysByPattern` function by using the `SCAN` command with a pipeline of `DEL` commands for more efficient key deletion.

-   [ ] **`health` Module:**
    -   [ ] Improve the performance of cache-related health checks to avoid iterating over all keys.

-   [ ] **`guild` Module:**
    -   [ ] Investigate the `GuildState` mutex for potential lock contention. If performance issues are identified, implement more granular locking.

### Priority 4: Code Quality and Maintainability

-   [ ] **`cache` Module:**
    -   [ ] Centralize all cache key generation logic within the `cache` module to ensure consistency and ease of maintenance.

-   [ ] **`app` Module:**
    -   [ ] Relocate the startup message logic from the `Run` method to the `reporting` or `startup` module.
    -   [ ] Move hardcoded boot messages from the `Run` method into a configuration file or a dedicated constants file.

-   [ ] **`reporting` Module:**
    -   [ ] Refactor the `PostFinalStatus` function into smaller, more focused functions to improve readability and maintainability.

-   [ ] **`cleanup` Module:**
    -   [ ] Add logging for any errors that are currently being ignored in the cleanup process.

### Priority 5: Configuration and Deployment

-   [ ] **`config` Module:**
    -   [ ] Simplify configuration management by consolidating the multiple JSON files into a single file or by using a library like [Viper](https://github.com/spf13/viper).

-   [ ] **`stt` Module:**
    -   [ ] Move the hardcoded Google Cloud Speech-to-Text configuration into the application's configuration files.
