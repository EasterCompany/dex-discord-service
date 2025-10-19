# Refactoring Plan

This document outlines a plan for refactoring the codebase to improve maintainability, code re-usability, and adherence to DRY standards.

## 1. Main Function (Completed)

*   **Area:** `main.go`
*   **Issue:** The `main` function is too long and acts as a central orchestrator for the entire application. This makes it difficult to read and maintain.
*   **Proposed Solution:** Break down the `main` function into smaller, more focused functions. For example, create separate functions for:
    *   Loading the configuration.
    *   Initializing the Discord session.
    *   Initializing the cache.
    *   Setting up the event handlers.
    *   Handling the OS signal for graceful shutdown.
*   **Benefits:** Improved readability, better separation of concerns, and easier testing of individual components.

## 2. Configuration Loading (Completed)

*   **Area:** `config/config.go`
*   **Issue:** The `LoadAllConfigs` function contains repetitive code for loading each configuration file. The default configurations are also hardcoded within this function.
*   **Proposed Solution:**
    *   Create a generic `loadConfig` function that takes a file path and a struct as arguments and handles the loading and decoding of the JSON file.
    *   Move the default configurations to separate `default.json` files for each configuration type (e.g., `discord.default.json`, `cache.default.json`, `bot.default.json`). The `loadConfig` function can then use these files to create the default configuration if the main config file does not exist.
*   **Benefits:** Reduced code duplication, better organization of default configurations, and easier management of configuration files.

## 3. Logging (Completed)

*   **Area:** `log/log.go`
*   **Issue:** The `log` package uses global state for the Discord session and channel ID, which is not ideal. It is also tightly coupled with the `discordgo` library.
*   **Proposed Solution:**
    *   Refactor the `log` package to be a struct that is initialized with the Discord session and channel ID. This will remove the global state and make the package more modular.
    *   Create a `Logger` interface that defines the logging methods (`Post`, `Error`, `Fatal`). The `log` package will implement this interface. This will decouple the rest of the application from the `discordgo` library and allow for easier testing and mocking of the logger.
*   **Benefits:** Improved testability, better separation of concerns, and reduced coupling.

## 4. Cache (Completed)

*   **Area:** `cache/cache.go`
*   **Issue:**
    *   The `LoadGuildState` function has a potential bug where it overwrites the `ActiveStreams` field with an empty map, effectively losing cached data.
    *   The `cleanKeysByPattern` function can be optimized to be more efficient.
*   **Proposed Solution:**
    *   Fix the `LoadGuildState` function to correctly merge the cached state with the new state, preserving the `ActiveStreams` data.
    *   Optimize the `cleanKeysByPattern` function by using a more efficient algorithm for pattern matching and key deletion.
*   **Benefits:** Bug fixes, improved performance, and more reliable caching.

## 5. Event Handling (Completed)

*   **Area:** `events/events.go`
*   **Issue:**
    *   The `guildStates` map is a global variable, leading to complex state management.
    *   The `joinVoice` and `handleAudioPacket` functions are too long and complex.
    *   There is code duplication and inconsistent error handling.
*   **Proposed Solution:**
    *   Refactor the state management to be more centralized and encapsulated. Create a `StateManager` struct that is responsible for managing the `guildStates` map. This will remove the global variable and make the state management more robust.
    *   Break down the `joinVoice` and `handleAudioPacket` functions into smaller, more focused functions.
    *   Create helper functions to reduce code duplication.
    *   Implement a consistent error handling strategy.
*   **Benefits:** Improved readability, better maintainability, reduced complexity, and more robust state management.

## 6. Cleanup (Completed)

*   **Area:** `cleanup/cleanup.go`
*   **Issue:** The `ClearChannel` function contains a hardcoded channel ID.
*   **Proposed Solution:** Move the channel ID to the `discord.json` configuration file.
*   **Benefits:** Better configurability and easier management of channel IDs.

## 7. Obsolete Code

*   **Area:** `store/store.go`
*   **Issue:** This package appears to contain obsolete code for cleaning up an old file-based storage system.
*   **Proposed Solution:** Remove the `store` package.
*   **Benefits:** Reduced codebase size and removal of unnecessary code.
