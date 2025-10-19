# Dexter Discord Interface

This document provides a comprehensive overview of the Dexter Discord Interface, a Go application designed to act as a systemd service. It serves as a guide for understanding the project's architecture, features, and operational procedures.

## Project Overview

The Dexter Discord Interface is a Discord bot that integrates with an internal AI tool named "Dexter". The bot's primary functions are to cache message logs from Discord and to provide real-time speech-to-text (STT) transcription of voice channels it is connected to.

## Core Features

### Message Caching

The application caches message logs from all channels the bot has access to. This includes both server channels and direct messages. The primary purpose of this feature is to prepare data for later use by the Dexter AI.

-   **Mechanism**: On startup, the bot fetches the last 50 messages from all accessible text channels and stores them in a Redis cache. It then continues to cache new messages as they are created.
-   **Storage**: Messages are stored in Redis lists, with each list representing a specific channel. The keys are formatted as `dex-discord-interface:messages:guild:<guild_id>:channel:<channel_id>` for server channels and `dex-discord-interface:messages:dm:<channel_id>` for direct messages.
-   **Maintenance**: The message cache is automatically cleaned on startup.

### Speech-to-Text (STT) Transcription

When the bot is in a voice channel, it automatically listens for users speaking and transcribes their speech in real-time.

-   **Process**:
    1.  A user with the appropriate permissions can summon the bot to a voice channel using the `!join` command.
    2.  When a user begins to speak, the bot creates a new Ogg audio stream and begins recording.
    3.  A message is posted in the designated transcription channel indicating that the user is speaking.
    4.  When the user stops speaking, the audio stream is closed and saved to the Redis cache with a configurable TTL.
    5.  The audio is then sent to the Google Cloud Speech-to-Text API for transcription.
    6.  Once the transcription is complete, the original message in the transcription channel is updated with the transcribed text.
    7.  The audio file is deleted from the cache after transcription.
-   **State Management**: The bot maintains a state for each guild, including active audio streams and user-to-SSRC mappings. This state is saved to the Redis cache, allowing the bot to potentially resume its state after a restart.

## Architecture

The application is composed of several packages, each with a specific responsibility.

-   **`main`**: The entry point of the application. It initializes the configuration, Discord session, logger, cache, and STT client. It also registers event handlers and orchestrates the startup and shutdown sequences.
-   **`config`**: Manages the application's configuration, which is loaded from multiple JSON files.
-   **`session`**: Manages the Discord session, including specifying the necessary intents for the bot to function correctly.
-   **`events`**: Handles events from the Discord Gateway, such as new messages, voice state updates, and user speaking events.
-   **`log`**: Implements a custom logger that sends messages to a designated Discord channel, providing real-time feedback on the bot's status.
-   **`cache`**: Provides an interface for interacting with the Redis cache, used for storing messages, guild states, and audio data.
-   **`guild`**: Defines the data structures for guild-specific state (`GuildState`) and user audio streams (`UserStream`).
-   **`stt`**: Contains the client for the Google Cloud Speech-to-Text API.
-   **`health`**: Provides functions for checking the status of the Discord connection and cache.
-   **`cleanup`**: Contains functions for performing cleanup tasks on startup, such as clearing channels and handling stale messages.
-   **`system`**: Provides functions for getting system metrics like CPU and memory usage.
-   **`interfaces`**: Defines interfaces for the database and STT modules, promoting loose coupling and testability.

### Systemd Service

The application is designed to run as a `systemd` service. The `scripts/install.sh` script sets up the service file and installs the application binary. The service is configured to restart automatically on failure.

## Dependencies

The application relies on several key Go modules:

-   **`github.com/bwmarrin/discordgo`**: The primary library for interacting with the Discord API.
-   **`github.com/redis/go-redis/v9`**: The client library for Redis, used for all caching purposes.
-   **`cloud.google.com/go/speech`**: The client library for the Google Cloud Speech-to-Text API.
-   **`github.com/pion/webrtc/v3`**: Used for handling the real-time audio streams from Discord.
-   **`github.com/shirou/gopsutil/v3`**: Used for retrieving system metrics.

## Scripts

The `/scripts` directory contains a collection of bash scripts for managing the application:

-   `build.sh`: Compiles the Go application.
-   `config.sh`: Creates boilerplate configuration files.
-   `install.sh`: Installs the application and sets up the systemd service.
-   `lint.sh`: Runs `golangci-lint` to format and lint the code.
-   `logs.sh`: Displays the latest logs from the systemd service.
-   `make.sh`: A master script that runs the linter, verifier, builder, and installer in sequence.
-   `restart.sh`: Restarts the systemd service.
-   `start.sh`: Starts the systemd service.
-   `stop.sh`: Stops the systemd service.
-   `test.sh`: Runs the application's tests.
-   `uninstall.sh`: Removes the application and systemd service.
-   `verify.sh`: Builds and runs the configuration verification tool.

## Development

To contribute to the development of the Dexter Discord Interface, follow these steps:

1.  **Lint**: Before committing any changes, run `./scripts/lint.sh` to ensure your code adheres to the project's coding standards.
2.  **Test**: Run `./scripts/test.sh` to execute the application's tests.
3.  **Build**: Use `./scripts/build.sh` to compile the application.
4.  **Verify**: If you make changes to the configuration, run `./scripts/verify.sh` to ensure the configuration is still valid.

## Installation

To install and run the Dexter Discord Interface, follow these steps:

1.  **Configure**: Run `./scripts/config.sh` to create the initial configuration files. Then, edit the files in `~/Dexter/config/` with your specific settings (bot token, channel IDs, etc.).
2.  **Build**: Run `./scripts/make.sh` to lint, test, build, and install the application. You will be prompted for your `sudo` password during the installation step.
3.  **Manage**: Use the `start.sh`, `stop.sh`, and `restart.sh` scripts to manage the service.
4.  **Logs**: Use the `logs.sh` script to view the service's logs.
