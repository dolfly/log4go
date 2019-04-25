package log4go

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// This log writer sends output to a file
type PanicFileLogWriter struct {
	LogCloser //for Elegant exit

	rec chan *LogRecord

	// The opened file
	filename     string
	baseFilename string // abs path
	file         *os.File

	// The logging format
	format string

	when        string // 'D', 'H', 'M'
	backupCount int    // If backupCount is > 0, when rollover is done,
	// no more than backupCount files are kept

	interval   int64
	suffix     string         // suffix of log file
	fileFilter *regexp.Regexp // for removing old log files

	rolloverAt    int64 // time.Unix()
	firstRollover bool  // the flag of first Rollover
}

// This is the FileLogWriter's output method
func (w *PanicFileLogWriter) LogWrite(rec *LogRecord) {
	if !LogWithBlocking {
		if len(w.rec) >= LogBufferLength {
			//            if WithModuleState {
			//                log4goState.Inc("ERR_TIMEFILE_LOG_OVERFLOW", 1)
			//            }

			return
		}
	}

	w.rec <- rec
}

//wait for dump all log and close chan
func (w *PanicFileLogWriter) Close() {
	w.WaitForEnd(w.rec)
	close(w.rec)
}

/* prepare according to "when"  */
func (w *PanicFileLogWriter) prepare() {
	var regRule string

	switch w.when {
	case "M":
		w.interval = 60
		w.suffix = "%Y-%m-%d_%H-%M"
		regRule = `^\d{4}-\d{2}-\d{2}_\d{2}-\d{2}$`
	case "H":
		w.interval = 60 * 60
		w.suffix = "%Y%m%d%H"
		regRule = `^\d{10}$`
	case "D", "MIDNIGHT":
		w.interval = 60 * 60 * 24
		w.suffix = "%Y-%m-%d"
		regRule = `^\d{4}-\d{2}-\d{2}$`
	default:
		// default is "D"
		w.interval = 60 * 60 * 24
		w.suffix = "%Y-%m-%d"
		regRule = `^\d{4}-\d{2}-\d{2}$`
	}
	w.fileFilter = regexp.MustCompile(regRule)

	fInfo, err := os.Stat(w.filename)

	var t time.Time
	if err == nil {
		t = fInfo.ModTime()
	} else {
		t = time.Now()
	}

	w.firstRollover = true
	w.rolloverAt = (t.Unix()/w.interval + 1) * w.interval
}

/*
* NewPanicFileLogWriter - creates a new PanicFileLogWriter
*
* PARAMS:
*   - fname: name of log file
*   - when:
*       "M", minute
*       "H", hour
*       "D", day
*       "MIDNIGHT", roll over at midnight
*   - backupCount: If backupCount is > 0, when rollover is done, no more than
*       backupCount files are kept - the oldest ones are deleted.
*
* RETURNS:
*   pointer to PanicFileLogWriter, if succeed
*   nil, if fail
 */
func NewPanicFileLogWriter(fname string, when string, backupCount int) *PanicFileLogWriter {
	when = strings.ToUpper(when)

	w := &PanicFileLogWriter{
		rec:         make(chan *LogRecord, LogBufferLength),
		filename:    fname,
		format:      "[%D %T] [%L] (%S) %M",
		when:        when,
		backupCount: backupCount,
	}

	return w.run(fname)
}

/* rename file to backup name   */
func (w *PanicFileLogWriter) run(fname string) *PanicFileLogWriter {
	//init LogCloser
	w.LogCloserInit()

	// get abs path
	if path, err := filepath.Abs(fname); err != nil {
		fmt.Fprintf(os.Stderr, "FileLogWriter(%q): %s\n", w.filename, err)
		return nil
	} else {
		w.baseFilename = path
	}

	// prepare for w.interval, w.suffix and w.fileFilter
	w.prepare()

	// open the file for the first time
	if err := w.intRotate(); err != nil {
		fmt.Fprintf(os.Stderr, "FileLogWriter(%q): %s\n", w.filename, err)
		return nil
	}

	go func() {
		defer func() {
			if w.file != nil {
				w.file.Close()
			}
		}()

		for {
			select {
			case rec, ok := <-w.rec:
				if !ok {
					return
				}

				if w.EndNotify(rec) {
					return
				}

				// Perform the write
				var err error
				if rec.Binary != nil {
					_, err = w.file.Write(rec.Binary)
				} else {
					_, err = fmt.Fprint(w.file, FormatLogRecord(w.format, rec))
				}
				if err != nil {
					fmt.Fprintf(os.Stderr, "FileLogWriter(%q): %s\n", w.filename, err)
					return
				}
			}
		}
	}()

	return w
}

func (w *PanicFileLogWriter) intRotate() error {
	if w.file != nil {
		w.file.Close()
	}

	//w.filename = w.baseFilename + "." + strftime.Format(w.suffix, time.Now())
	fd, err := os.OpenFile(w.filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	w.file = fd
	if os.Getenv("LOGGER_MODE") != "debug" {
		syscall.Dup2(int(fd.Fd()), 1)
		syscall.Dup2(int(fd.Fd()), 2)
	}
	return nil
}

// Set the logging format (chainable).  Must be called before the first log
// message is written.
func (w *PanicFileLogWriter) SetFormat(format string) *PanicFileLogWriter {
	w.format = format
	return w
}
