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

func init() {
	slackChannel = os.Getenv("SLACK_CHANNEL")

	slackToken := os.Getenv("SLACK_TOKEN")
	if slackToken != "" && slackChannel != "" {
		slackApi = slack.New(slackToken)
	}
}

func PushSlackf(format string, args ...interface{}) chan struct{} {
	done := make(chan struct{}, 1)
	go func() {
		if slackApi == nil {
			done <- struct{}{}
			return
		}

		message := fmt.Sprintf(format, args...)

		block := slack.NewContextBlock("", slack.NewTextBlockObject("mrkdwn", message, false, false))

		blocks := slack.MsgOptionBlocks(block)

		_, _, resp, e := slackApi.SendMessage(slackChannel, slack.MsgOptionText("", false), blocks)

		if e != nil {
			log.Println(e, resp)
		}
		done <- struct{}{}
	}()
	return done
}

func DeploySuccessSlack(dir string, commands []string) chan struct{} {
	server := os.Getenv("SERVER")

	return PushSlackf("*`%s` 部署成功* :stars: \n\n> 应用 `%s` \n\n ```%s```", server, dir, strings.Join(commands, "\n"))
}
