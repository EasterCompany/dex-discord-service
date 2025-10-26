# Backlog

This document contains a list of features and improvements that are planned for future development.

## Local Speech-to-Text (STT)

**Description:**

Implement a local Speech-to-Text (STT) solution to replace the current Google Cloud STT API. This would involve using a local STT model (e.g., Whisper, DeepSpeech) to transcribe audio from voice channels.

**Motivation:**

The primary motivation for this change is to reduce the latency of transcriptions. By processing audio locally, we can eliminate the network latency of sending data to Google Cloud and waiting for a response.

**Considerations:**

*   **Complexity:** Implementing and maintaining a local STT pipeline is significantly more complex than using a cloud API.
*   **Resource Intensive:** STT models can be very demanding on system resources, particularly the GPU.
*   **Development Time:** This would be a significant undertaking.

**Status:**

This feature is currently backlogged. We will revisit it in the future if the latency of the Google Cloud STT API becomes a significant issue.
