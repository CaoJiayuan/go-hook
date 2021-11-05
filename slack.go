package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/slack-go/slack"
)

var slackApi *slack.Client

var slackChannel string

func initSlack() {
	slackChannel = os.Getenv("SLACK_CHANNEL")

	slackToken := os.Getenv("SLACK_TOKEN")
	fmt.Printf("slack init [%s] [%s]\n", slackChannel, slackToken)
	if slackToken != "" && slackChannel != "" {
		slackApi = slack.New(slackToken)
	}
}

func PushSlackf(format string, logger *log.Logger, notifyMsg string, args ...interface{}) chan struct{} {
	done := make(chan struct{}, 1)
	go func() {
		if slackApi == nil {
			done <- struct{}{}
			return
		}

		message := fmt.Sprintf(format, args...)

		block := slack.NewContextBlock("", slack.NewTextBlockObject("mrkdwn", message, false, false))

		blocks := slack.MsgOptionBlocks(block)

		_, _, e := slackApi.PostMessage(slackChannel, slack.MsgOptionText(notifyMsg, false), blocks)

		if e != nil {
			outputAndLog(logger, e)
		}
		done <- struct{}{}
	}()
	return done
}

func DeploySuccessSlack(dir string, commands []string, logger *log.Logger, service ...string) chan struct{} {
	server := os.Getenv("SERVER")

	var s string
	if len(service) > 0 && service[0] != "" {
		s = fmt.Sprintf(": %s", service[0])
	}
	return PushSlackf("*`%s` 部署成功* :stars: \n\n> 应用 `%s` \n\n ```%s```", logger,
		fmt.Sprintf("[%s] 部署成功 (%s%s)", server, dir, s),
		server, dir, strings.Join(commands, "\n"))
}
