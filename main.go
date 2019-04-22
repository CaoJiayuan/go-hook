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
)

var (
	path string
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
		return execCommands(string(dir), []string{
			"git pull",
		}, logger)
	}
	return false
}

func handleLaravelDeploy(dir []byte, logger *log.Logger, extra []byte) bool {
	if len(dir) > 0 {
		logger.Println(fmt.Sprintf("laravel [%s] deploy", dir))
		fmt.Println(fmt.Sprintf("laravel [%s] deploy", dir))
		commands := []string{
			"git pull",
			"composer install --ignore-platform-reqs",
		}

		if len(extra) > 0 {
			commands = append(commands, strings.Split(string(extra), "&")...)
		}

		return execCommands(string(dir), commands, logger)
	}
	return false
}

func main1() {
	e := make(chan int)
	q := &Queue{}

	q.Run(e)
}

func main() {
	path = current()
	godotenv.Load(path + "/.env")
	e := make(chan int)
	q := NewQueue()

	q.Run(e)

	defer func() {
		e <- 1
	}()

	apiToken := "123456"

	err := fasthttp.ListenAndServe(":8181", func(ctx *fasthttp.RequestCtx) {
		query := ctx.QueryArgs()

		ty := query.Peek("type")

		token := string(query.Peek("token"))

		if apiToken != token {
			fmt.Fprint(ctx, "Access denied")
			ctx.Response.SetStatusCode(403)
		} else  {
			if len(ty) > 0 {
				s := string(ty)
				dir := query.Peek("dir")
				switch s {
				case "git":
					q.Push(func(logger *log.Logger) bool {
						return handleGitDeploy(dir, logger)
					})
				case "laravel":
					q.Push(func(logger *log.Logger) bool {
						return handleLaravelDeploy(dir, logger, query.Peek("extra"))
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
