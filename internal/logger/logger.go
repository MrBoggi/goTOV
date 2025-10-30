package logger

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
)

func New() zerolog.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	return log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
}
