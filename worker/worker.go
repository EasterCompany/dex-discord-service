package worker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/EasterCompany/dex-discord-interface/guild"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/bwmarrin/discordgo"
)

// TranscriptionJob holds all the necessary data for a single transcription task.
type TranscriptionJob struct {
	Ctx     context.Context
	Session *discordgo.Session
	Reader  io.Reader
	Stream  *guild.UserStream
	DB      interfaces.Database
	STT     interfaces.STT
}

// WorkerPool manages a pool of workers and a queue of jobs.
type WorkerPool struct {
	JobQueue   chan TranscriptionJob
	MaxWorkers int
}

// New creates a new WorkerPool.
func New(maxWorkers, queueSize int) *WorkerPool {
	return &WorkerPool{
		JobQueue:   make(chan TranscriptionJob, queueSize),
		MaxWorkers: maxWorkers,
	}
}

// Start creates and starts the worker goroutines.
func (wp *WorkerPool) Start() {
	for i := 1; i <= wp.MaxWorkers; i++ {
		go wp.worker(i)
	}
}

// Submit adds a new job to the job queue.
func (wp *WorkerPool) Submit(job TranscriptionJob) {
	wp.JobQueue <- job
}

// worker is a goroutine that continuously processes jobs from the JobQueue.
func (wp *WorkerPool) worker(id int) {
	for job := range wp.JobQueue {
		processTranscription(job)
	}
}

// processTranscription contains the logic for a single transcription task.
// This was formerly the transcribeStream function.
func processTranscription(job TranscriptionJob) {
	transcriptChan := make(chan string)
	errChan := make(chan error, 1)

	go job.STT.StreamingTranscribe(job.Ctx, job.Reader, transcriptChan, errChan)

	var finalTranscript strings.Builder
	for {
		select {
		case transcript, ok := <-transcriptChan:
			if !ok {
				stopTime := time.Now()
				finalTranscriptStr := strings.TrimSpace(finalTranscript.String())

				if finalTranscriptStr != "" {
					if err := job.DB.LogTranscription(job.Stream.GuildID, job.Stream.ChannelID, job.Stream.User.Username, finalTranscriptStr); err != nil {
						logger.Error(fmt.Sprintf("logging transcription for user %s", job.Stream.User.Username), err)
					}
				}

				finalContent := fmt.Sprintf("**%s:** %s\n*(`%s` to `%s`)*", job.Stream.User.Username, finalTranscriptStr, job.Stream.StartTime.Format("15:04:05"), stopTime.Format("15:04:05 MST"))
				if _, err := job.Session.ChannelMessageEdit(job.Stream.Message.ChannelID, job.Stream.Message.ID, finalContent); err != nil {
					logger.Error(fmt.Sprintf("editing final transcription message for user %s", job.Stream.User.Username), err)
				}
				return
			}
			finalTranscript.WriteString(transcript)
			interimContent := fmt.Sprintf("`%s:` %s...", job.Stream.User.Username, finalTranscript.String())
			if len(interimContent) > 2000 {
				interimContent = interimContent[:1997] + "..."
			}
			if _, err := job.Session.ChannelMessageEdit(job.Stream.Message.ChannelID, job.Stream.Message.ID, interimContent); err != nil {
				// We don't log this error to avoid spamming the log channel with potentially frequent errors
				// that occur during rapid interim updates. The final message edit is the important one to log.
			}

		case err := <-errChan:
			logger.Error(fmt.Sprintf("transcription for user %s", job.Stream.User.Username), err)
			job.Session.ChannelMessageEdit(job.Stream.Message.ChannelID, job.Stream.Message.ID, fmt.Sprintf("Error during transcription for `%s`.", job.Stream.User.Username))
			return
		case <-job.Ctx.Done():
			return
		}
	}
}
