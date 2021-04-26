package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/enorith/supports/file"
	"github.com/joho/godotenv"
	"github.com/sevlyar/go-daemon"
	"github.com/valyala/fasthttp"
)

var (
	path string
)

const (
	MIME = "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
)

var sLogger *log.Logger

type Job func(logger *log.Logger) bool

type Queue struct {
	jobs   []Job
	m      *sync.Mutex
	wg     *sync.WaitGroup
	logger *log.Logger
}

func (q *Queue) Push(j Job) {
	//q.m.Lock()
	//defer q.m.Lock()
	q.jobs = append(q.jobs, j)
}

func (q *Queue) Shift() (j Job) {
	if len(q.jobs) < 1 {
		return nil
	}
	q.m.Lock()
	defer q.m.Unlock()
	j, q.jobs = q.jobs[0], q.jobs[1:]
	return
}

func (q *Queue) Run(exit chan int, logger *log.Logger) {
	go func() {
		t := time.NewTicker(time.Millisecond * 200)
		for {
			select {
			case <-t.C:
				q.handle(logger)
			case <-exit:
				t.Stop()
				return
			}
		}
	}()
}

func (q *Queue) handle(logger *log.Logger) {
	j := q.Shift()
	if j != nil {
		q.wg.Add(1)
		go func(l *log.Logger) {
			defer q.wg.Done()
			j(l)
		}(logger)
		q.wg.Wait()
	}
}

func getLogger(path string) *log.Logger {
	if sLogger != nil {
		return sLogger
	}

	logFile := os.Getenv("HOOK_LOG_FILE")

	if len(logFile) < 1 {
		logFile = path + "/deploy.log"
	}

	file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0775)

	if err != nil {
		fmt.Println(err)
	}

	sLogger := log.New(file, "", log.LstdFlags)

	return sLogger
}

func current() string {
	path, _ := filepath.Abs(".")

	return path
}

func outputAndLog(logger *log.Logger, output interface{}) {
	fmt.Println(output)
	logger.Println(output)
}

func execCommands(dir string, commands []string, logger *log.Logger) bool {
	curr := current()
	e := os.Chdir(dir)
	if e != nil {
		fmt.Println(e)
		return false
	}
	outputAndLog(logger, fmt.Sprintf("dir [%s]", dir))

	for _, v := range commands {
		outputAndLog(logger, fmt.Sprintf("exec [%s]", v))

		partials := strings.Split(v, " ")
		var args []string
		if len(partials) > 1 {
			args = partials[1:]
		}
		cmd := exec.Command(partials[0], args...)

		stdout, err := cmd.StdoutPipe()

		if err != nil {
			logger.Println(err)
			fmt.Println(err)
			return false
		}

		cmdErr := cmd.Start()
		if cmdErr != nil {
			logger.Println(cmdErr)
			fmt.Println(cmdErr)
			return false
		}

		reader := bufio.NewReader(stdout)

		for {
			line, err2 := reader.ReadString('\n')
			if io.EOF == err2 {
				break
			}
			if err2 != nil {
				fmt.Println(err2)
				logger.Println(err2)
				break
			}

			logger.Println(line)
			fmt.Println(line)
		}

		cmd.Wait()
	}

	os.Chdir(curr)
	return true
}

func NewQueue() *Queue {
	return &Queue{jobs: []Job{}, m: &sync.Mutex{}, wg: &sync.WaitGroup{}}
}

func handleGitDeploy(dir []byte, logger *log.Logger) bool {
	if len(dir) > 0 {
		logger.Println(fmt.Sprintf("git [%s] deploy", dir))
		fmt.Println(fmt.Sprintf("git [%s] deploy", dir))
		commands := []string{
			"git pull",
		}
		s := string(dir)
		result := execCommands(s, commands, logger)
		go sendEmail(s, commands, logger)
		return result
	}
	return false
}

func handleComposeDeploy(dir []byte, logger *log.Logger, service []byte) bool {
	if len(dir) > 0 {
		logger.Println(fmt.Sprintf("git [%s] compose", dir))
		fmt.Println(fmt.Sprintf("git [%s] compose", dir))
		var commands = []string{}

		if len(service) > 0 {
			srv := string(service)
			commands = []string{
				"docker-compose pull " + srv,
				"docker-compose stop " + srv,
				"docker-compose rm --force " + srv,
			}
		} else {
			commands = []string{
				"docker-compose pull",
				"docker-compose down",
			}
		}
		commands = append(commands, "docker-compose up -d")
		s := string(dir)
		result := execCommands(s, commands, logger)
		go sendEmail(s, commands, logger)
		return result
	}
	return false
}

func handleLaravelDeploy(dir []byte, logger *log.Logger, extra []byte) bool {
	if len(dir) > 0 {
		logger.Println(fmt.Sprintf("laravel [%s] deploy", dir))
		fmt.Println(fmt.Sprintf("laravel [%s] deploy", dir))

		composerPath := os.Getenv("COMPOSER_PATH")

		if len(composerPath) < 1 {
			composerPath = "/usr/local/bin/composer"
		}
		dirS := string(dir)

		beforeHook := filepath.Join(dirS, "before_hook.sh")
		be, _ := file.PathExists(beforeHook)

		var commands []string

		if be {
			commands = []string{
				beforeHook,
				"git pull",
				"php " + composerPath + " install --ignore-platform-reqs",
			}
		} else {
			commands = []string{
				"git pull",
				"php " + composerPath + " install --ignore-platform-reqs",
			}
		}

		afterHook := filepath.Join(dirS, "after_hook.sh")
		ae, _ := file.PathExists(afterHook)
		if ae {
			commands = append(commands, afterHook)
		}

		if len(extra) > 0 {
			commands = append(commands, strings.Split(string(extra), ",")...)
		}

		result := execCommands(dirS, commands, logger)
		go sendEmail(dirS, commands, logger)

		return result
	}
	return false
}

func handleNpmDeploy(dir []byte, logger *log.Logger, env []byte, extra []byte) bool {
	if len(dir) > 0 {
		outputAndLog(logger, fmt.Sprintf("npm [%s] deploy", dir))
		if len(env) == 0 {
			env = []byte("uat")
		}
		commands := []string{
			"git pull",
			"npm install --unsafe-perm",
			"npm run " + string(env),
		}

		if len(extra) > 0 {
			commands = append(commands, strings.Split(string(extra), ",")...)
		}

		dirS := string(dir)
		result := execCommands(dirS, commands, logger)
		go sendEmail(dirS, commands, logger)
		return result
	}
	return false
}

type EmailData struct {
	Commands []string
	Server   string
	Dir      string
}

func sendEmail(dir string, commands []string, logger *log.Logger) bool {
	if os.Getenv("SEND_EMAIL") != "1" {
		return false
	}
	from := os.Getenv("SMTP_EMAIL")
	pass := os.Getenv("SMTP_PASS")
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	auth := smtp.PlainAuth(
		"",
		from,
		pass,
		host,
	)
	to := os.Getenv("SMTP_TO")

	var buf bytes.Buffer
	temp, tErr := template.ParseFiles(filepath.Join(path, "email.temp.html"))

	if tErr != nil {
		outputAndLog(logger, "sended email error, no mail template")
		return false
	}

	temp.Execute(&buf, EmailData{
		Commands: commands,
		Server:   os.Getenv("SERVER"),
		Dir:      dir,
	})
	msg := "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: Deploy message\r\n" +
		MIME + "\r\n" +
		buf.String()

	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	outputAndLog(logger, "sending email")
	err := smtp.SendMail(
		host+":"+port,
		auth,
		from,
		strings.Split(to, ","),
		[]byte(msg),
	)
	if err != nil {
		fmt.Println(err)
		logger.Println(err)
		return false
	} else {
		outputAndLog(logger, "send success")
		return true
	}
}

func getPidFile() string {
	pidFile := os.Getenv("HOOK_PID_FILE")

	if len(pidFile) < 1 {
		pidFile = "/var/run/go-hook.pid"
	}

	return pidFile
}
func getPid() (int, error) {
	data, err := ioutil.ReadFile(getPidFile())

	if err != nil {
		return 0, err
	}
	pid, e := strconv.Atoi(string(data))

	if e != nil {
		return 0, e
	}
	return pid, nil
}

func daemonize(logger *log.Logger, path string) {
	pidFile := getPidFile()

	ctx := &daemon.Context{
		PidFileName: pidFile,
		PidFilePerm: 0644,
		LogFilePerm: 0644,
		WorkDir:     current(),
	}

	d, err := ctx.Reborn()
	if err != nil {
		outputAndLog(logger, "Daemon process can not run")
		outputAndLog(logger, err)
		return
	}
	if d != nil {
		outputAndLog(logger, "Hook process running at daemon mode")
		return
	}
	defer ctx.Release()
	start(logger, path)
}

func start(logger *log.Logger, dir string) {
	outputAndLog(logger, fmt.Sprintf("Hook start in [%s]", dir))
	e := make(chan int)
	q := NewQueue()

	q.Run(e, logger)

	defer func() {
		e <- 1
	}()

	apiToken := os.Getenv("API_TOKEN")

	port := os.Getenv("HOOK_PORT")
	if len(port) < 1 {
		port = "8181"
	}

	outputAndLog(logger, fmt.Sprintf("Hook server started [0.0.0.0:%s], api token [%s]", port, apiToken))

	err := fasthttp.ListenAndServe(":"+port, func(ctx *fasthttp.RequestCtx) {
		query := ctx.QueryArgs()

		ty := query.Peek("type")

		token := string(query.Peek("token"))

		if apiToken != token {
			fmt.Fprint(ctx, "Access denied")
			ctx.Response.SetStatusCode(403)
		} else {
			if len(ty) > 0 {
				s := string(ty)
				dir := query.Peek("dir")
				extra := query.Peek("extra")
				switch s {
				case "git":
					q.Push(func(logger *log.Logger) bool {
						return handleGitDeploy(dir, logger)
					})
				case "laravel":
					q.Push(func(logger *log.Logger) bool {
						return handleLaravelDeploy(dir, logger, extra)
					})
				case "npm":
					env := query.Peek("env")
					q.Push(func(logger *log.Logger) bool {
						return handleNpmDeploy(dir, logger, env, extra)
					})
				case "compose":
					service := query.Peek("service")
					q.Push(func(logger *log.Logger) bool {
						return handleComposeDeploy(dir, logger, service)
					})
				}
			}
			fmt.Fprint(ctx, "OK")
		}
	})

	if err != nil {
		e <- 1
		panic(err)
	}
}

func startProcess(daemon bool, logger *log.Logger) {
	if daemon {
		daemonize(logger, path)
	} else {
		start(logger, path)
	}
}

func stopProcess() error {
	pid, err := getPid()
	if pid > 0 {
		syscall.Kill(pid, syscall.SIGTERM)
		syscall.Unlink(getPidFile())
		fmt.Println(fmt.Sprintf("Hook process[%d] killed", pid))
	} else {
		fmt.Println(err)
	}
	return err
}

func main() {
	d := flag.Bool("d", false, "run as daemon")
	flag.Parse()
	file, _ := filepath.Abs(os.Args[0])
	daemon := *d
	path = filepath.Dir(file)

	var cmd = "start"
	if len(os.Args) > 1 {
		if os.Args[1] == "-d" {
			cmd = "start"
			daemon = true
		} else {
			cmd = os.Args[1]
			if len(os.Args) > 2 {
				daemon = os.Args[2] == "-d"
			}
		}
	}

	godotenv.Load(path + "/.env")

	logger := getLogger(path)

	switch cmd {
	case "start":
		startProcess(daemon, logger)
		break
	case "restart":
		stopProcess()
		startProcess(daemon, logger)
		break
	case "stop":
		pid, err := getPid()
		if pid > 0 {
			syscall.Kill(pid, syscall.SIGTERM)
			syscall.Unlink(getPidFile())
			fmt.Println(fmt.Sprintf("Hook process[%d] killed", pid))
		} else {
			fmt.Println(err)
		}
		break
	case "pid":
		pid, err := getPid()
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println(fmt.Sprintf("Hook process running at pid [%d]", pid))
		}
	}

}
