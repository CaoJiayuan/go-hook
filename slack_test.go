package main

import (
	"log"
	"testing"

	"github.com/joho/godotenv"
)

func TestSlackMessage(t *testing.T) {
	godotenv.Load()
	initSlack()
	<-DeploySuccessSlack("/home/www/test", []string{"git pull", "composer install"}, log.Default(), "app")
}
