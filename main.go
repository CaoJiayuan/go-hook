package main

import (
	"github.com/valyala/fasthttp"
	"os"
	"path/filepath"
	"os/exec"
	"strings"
	"fmt"
	"io"
	"bufio"
	"sync"
	"time"
	"log"
	"github.com/joho/godotenv"
	"net/smtp"
	"html/template"
	"bytes"
)

var (
	path string
)

const (
	MIME = "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
)

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

func (q *Queue) Run(exit chan int) {
	go func() {
		t := time.NewTicker(time.Millisecond * 200)
		file, _ := os.OpenFile(path+"/deploy.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0775)
		logger := log.New(file, "", log.LstdFlags)
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

func current() string {
	path, _ := filepath.Abs(".")

	return path
}
func execCommands(dir string, commands []string, logger *log.Logger) bool {
	curr := current()
	e := os.Chdir(dir)
	if e != nil {
		fmt.Println(e)
		return false
	}
	fmt.Println(fmt.Sprintf("dir [%s]", dir))

	for _, v := range commands {
		fmt.Println(fmt.Sprintf("exec [%s]", v))

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
			if err2 != nil || io.EOF == err2 {
				fmt.Println(err2)
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
		go sendEmail(s, commands)
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

		commands := []string{
			"git pull",
			"php " + composerPath + " install --ignore-platform-reqs",
		}

		if len(extra) > 0 {
			commands = append(commands, strings.Split(string(extra), ",")...)
		}

		dirS := string(dir)
		result := execCommands(dirS, commands, logger)
		go sendEmail(dirS, commands)

		return result
	}
	return false
}

type EmailData struct {
	Commands []string
	Server   string
	Dir      string
}

func sendEmail(dir string, commands []string) bool {
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
		fmt.Println("sended email error, no mail template")
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
	fmt.Println("sending email")
	err := smtp.SendMail(
		host+":"+port,
		auth,
		from,
		strings.Split(to, ","),
		[]byte(msg),
	)
	if err != nil {
		fmt.Println(err)
		return false
	} else {
		fmt.Println("send success")
		return true
	}
}

func main() {
	file, _ := filepath.Abs(os.Args[0])

	path = filepath.Dir(file)
	godotenv.Load(path + "/.env")

	fmt.Println(fmt.Sprintf("Loading .env in [%s]", path))
	e := make(chan int)
	q := NewQueue()

	q.Run(e)

	defer func() {
		e <- 1
	}()

	apiToken := os.Getenv("API_TOKEN")

	port := os.Getenv("HOOT_PORT")
	if len(port) < 1 {
		port = "8181"
	}

	fmt.Println(fmt.Sprintf("Hook server started [0.0.0.0:%s], api token [%s]", port, apiToken))

	err := fasthttp.ListenAndServe(":" + port, func(ctx *fasthttp.RequestCtx) {
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
