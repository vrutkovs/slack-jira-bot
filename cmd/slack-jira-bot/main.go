package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"

	eventrouter "github.com/vrutkovs/slack-jira-bot/pkg/slack/events/router"
	interactionrouter "github.com/vrutkovs/slack-jira-bot/pkg/slack/interactions/router"

	"github.com/vrutkovs/slack-jira-bot/pkg/jira"
)

type options struct {
	port int

	logLevel               string
	gracePeriod            time.Duration
	instrumentationOptions prowflagutil.InstrumentationOptions
	jiraOptions            prowflagutil.JiraOptions

	slackTokenPath         string
	slackAppTokenPath      string
	slackSigningSecretPath string
	jiraProject            string
}

func (o *options) Validate() error {
	_, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		return fmt.Errorf("invalid --log-level: %w", err)
	}

	if o.slackTokenPath == "" {
		return fmt.Errorf("--slack-token-path is required")
	}

	if o.slackAppTokenPath == "" {
		return fmt.Errorf("--slack-app-token-path is required")
	}

	if o.slackSigningSecretPath == "" {
		return fmt.Errorf("--slack-signing-secret-path is required")
	}

	if o.jiraProject == "" {
		return fmt.Errorf("--jira-project is required")
	}

	for _, group := range []flagutil.OptionGroup{&o.instrumentationOptions, &o.jiraOptions} {
		if err := group.Validate(false); err != nil {
			return err
		}
	}

	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.IntVar(&o.port, "port", 8888, "Port to listen on.")

	fs.StringVar(&o.logLevel, "log-level", "info", "Level at which to log output.")
	fs.DurationVar(&o.gracePeriod, "grace-period", 180*time.Second, "On shutdown, try to handle remaining events for the specified duration. ")

	for _, group := range []flagutil.OptionGroup{&o.instrumentationOptions, &o.jiraOptions} {
		group.AddFlags(fs)
	}

	fs.StringVar(&o.slackTokenPath, "slack-token-path", "", "Path to the file containing the Slack token to use.")
	fs.StringVar(&o.slackAppTokenPath, "slack-app-token-path", "", "Path to the file containing the Slack app token to use.")
	fs.StringVar(&o.slackSigningSecretPath, "slack-signing-secret-path", "", "Path to the file containing the Slack signing secret to use.")
	fs.StringVar(&o.jiraProject, "jira-project", "", "Jira project name.")

	if err := fs.Parse(args); err != nil {
		logrus.WithError(err).Fatal("Could not parse args.")
	}
	return o
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}
	level, _ := logrus.ParseLevel(o.logLevel)
	logrus.SetLevel(level)

	if err := secret.Add(o.slackTokenPath, o.slackAppTokenPath, o.slackSigningSecretPath); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	jiraClient, err := o.jiraOptions.Client()
	if err != nil {
		logrus.WithError(err).Fatal("Could not initialize Jira client.")
	}

	slackClient := slack.New(string(secret.GetSecret(o.slackTokenPath)), slack.OptionAppLevelToken(string(secret.GetSecret(o.slackAppTokenPath))))
	issueFiler, err := jira.NewIssueFiler(slackClient, jiraClient.JiraClient(), o.jiraProject)
	if err != nil {
		logrus.WithError(err).Fatal("Could not initialize Jira issue filer.")
	}

	logger := logrus.WithField("api", "events")

	socketClient := socketmode.New(slackClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func(ctx context.Context, client *slack.Client, socketClient *socketmode.Client) {
		for {
			select {
			case <-ctx.Done():
				log.Println("Shutting down socketmode listener")
				return
			case event := <-socketClient.Events:
				switch event.Type {
				case socketmode.EventTypeEventsAPI:
					eventData, ok := event.Data.(slackevents.EventsAPIEvent)
					if !ok {
						log.Printf("Could not type cast the event to the EventsAPIEvent: %v\n", event)
						continue
					}
					socketClient.Ack(*event.Request)
					eventrouter.ForEvents(slackClient).Handle(&eventData, logger)
				case socketmode.EventTypeInteractive:
					interactionData, ok := event.Data.(slack.InteractionCallback)
					if !ok {
						log.Printf("Could not type cast the event to the InteractionCallback: %v\n", event)
						continue
					}
					payload, err := interactionrouter.ForModals(issueFiler, slackClient).Handle(&interactionData, logger)
					if err != nil {
						log.Printf("error building payload: %v\n", payload)
						continue
					}
					socketClient.Ack(*event.Request, payload)
				}
			}
		}
	}(ctx, slackClient, socketClient)

	socketClient.Run()
}
