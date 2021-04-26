package main

import (
	"log"
	"os"
	"testing"
)

func TestSlackSuccess(t *testing.T) {
	initSlack()

	slackChannel = os.Getenv("SLACK_CHANNEL")

	slackToken := os.Getenv("SLACK_TOKEN")

	t.Log(slackChannel)
	t.Log(slackToken)
	<-DeploySuccessSlack("/home/www/test", []string{"git pull", "composer install"}, log.Default())
}
