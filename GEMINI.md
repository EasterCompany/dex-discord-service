# Dexter Discord Interface

This document provides a comprehensive overview of the Dexter Discord Interface, a Go application designed to act as a systemd service. It serves as a guide for understanding the project's architecture, features, and operational procedures.

## Project Overview

The Dexter Discord Interface is a Discord bot that integrates with an internal AI tool named "Dexter". The bot's primary functions are to cache message logs from Discord, provide real-time speech-to-text (STT) transcription of voice channels, and to interact with users through a large language model (LLM).

## Core Features

### Message Caching

The application caches message logs from all channels the bot has access to. This includes both server channels and direct messages. The primary purpose of this feature is to prepare data for later use by the Dexter AI.

-   **Mechanism**: On startup, the bot fetches the last 50 messages from all accessible text channels and stores them in a Redis cache. It also proactively caches the last 50 messages from all DM channels it has previously interacted with. It then continues to cache new messages as they are created.
-   **Storage**: Messages are stored in Redis lists, with each list representing a specific channel. The keys are formatted as `dex-discord-interface:messages:server:<server_id>:channel:<channel_id>` for server channels and `dex-discord-interface:messages:dm:<channel_id>` for direct messages. A Redis set with the key `dex-discord-interface:dm_channels` is used to keep track of all DM channels the bot has interacted with.
-   **Maintenance**: The audio cache is automatically cleaned on startup. The message cache is persistent and is not cleaned on startup.

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
-   **State Management**: The bot maintains a state for each server, including active audio streams and user-to-SSRC mappings. This state is saved to the Redis cache, allowing the bot to potentially resume its state after a restart.

### LLM-based Chat

The bot can be configured to interact with users in text channels using a large language model.

-   **Mechanism**: The bot listens for messages in channels and, based on a set of rules, decides whether to engage in conversation. When it does, it uses the conversation history and a predefined persona to generate a response.
-   **Persona**: The bot's personality and behavior are defined in a `persona.json` file, which allows for detailed customization of its identity, communication style, and rules of engagement.

## User Guide

Interacting with the Dexter bot is simple. Here are the basic commands and features:

-   **`!join`**: Type `!join` in a text channel while you are in a voice channel on the same server. The bot will join your voice channel and begin automatically transcribing anyone who speaks.
-   **`!leave`**: Type `!leave` in a text channel on the same server where the bot is active in a voice channel. The bot will disconnect from the voice channel.
-   **Automatic Transcription**: When the bot is in a voice channel, it will post messages in the designated transcription channel, updating them in real-time with the transcribed speech of users.
-   **Chatting with the Bot**: In channels where the bot is active, you can chat with it by mentioning its name or by having a conversation that it decides to engage in.

## Architecture

The application is composed of several packages, each with a specific responsibility.

-   **`main`**: The entry point of the application. It initializes and runs the application.
-   **`app`**: The core of the application. It contains the main application struct and manages the application's lifecycle.
-   **`config`**: Manages the application's configuration, which is loaded from multiple JSON files.
-   **`session`**: Manages the Discord session, including specifying the necessary intents for the bot to function correctly.
-   **`events`**: Handles events from the Discord Gateway, such as new messages, voice state updates, and user speaking events.
-   **`log`**: Implements a custom logger that sends messages to a designated Discord channel, providing real-time feedback on the bot's status.
-   **`cache`**: Provides an interface for interacting with the Redis cache, used for storing messages, server states, and audio data.
-   **`guild`**: Defines the data structures for server-specific state (`GuildState`) and user audio streams (`UserStream`). The name of this package is a legacy artifact from the Discord API, but it manages server-specific state.
-   **`stt`**: Contains the client for the Google Cloud Speech-to-Text API.
-   **`llm`**: Contains the client and logic for interacting with the LLM.
-   **`health`**: Provides functions for checking the status of the Discord connection and cache.
-   **`cleanup`**: Contains functions for performing cleanup tasks on startup, such as clearing channels and handling stale messages.
-   **`startup`**: Manages the startup sequence of the application.
-   **`reporting`**: Manages the generation of the status report on startup.
-   **`system`**: Provides functions for getting system metrics like CPU and memory usage.
-   **`interfaces`**: Defines interfaces for the STT and LLM modules, promoting loose coupling and testability.

### Systemd Service

The application is designed to run as a `systemd` service. The `scripts/install.sh` script sets up the service file and installs the application binary. The service is configured to restart automatically on failure.

## Dependencies

The application relies on several key Go modules:

-   **`github.com/bwmarrin/discordgo`**: The primary library for interacting with the Discord API.
-   **`github.com/redis/go-redis/v9`**: The client library for Redis, used for all caching purposes.
-   **`cloud.google.com/go/speech`**: The client library for the Google Cloud Speech-to-Text API.
-   **`github.com/pion/webrtc/v3`**: Used for handling the real-time audio streams from Discord.
-   **`github.com/shirou/gopsutil/v3`**: Used for retrieving system metrics.

### Discord Intents

The bot requires the following Discord intents to be enabled:

-   `IntentGuilds`
-   `IntentGuildMessages`
-   `IntentGuildVoiceStates`
-   `IntentGuildMembers`
-   `IntentGuildPresences`
-   `IntentDirectMessages`
-   `IntentMessageContent`

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

## Developer Notes

This section contains notes for developers working on this project.

### Areas of Complexity

-   **`events/voice.go`**: The logic for handling voice data is complex. It involves managing multiple concurrent streams, handling RTP packets, and writing to Ogg files. This file requires careful attention when making changes.
-   **LLM Response Parsing**: The current implementation for parsing the LLM's XML-based response in `llm/stream.go` is based on regular expressions and string manipulation. This is brittle and could break if the LLM's output format changes. A more robust XML parsing solution should be considered.

### Future Improvements

-   **Testing**: The project currently lacks a comprehensive test suite. Adding unit and integration tests would significantly improve the code quality and make future development safer.
-   **Configuration Management**: The current configuration system, which relies on multiple JSON files, could be simplified by using a library like [Viper](https://github.com/spf13/viper).
-   **Error Handling**: There are many places in the code where errors are ignored. While some of these may be non-critical, it would be beneficial to log them at a debug level to aid in troubleshooting.