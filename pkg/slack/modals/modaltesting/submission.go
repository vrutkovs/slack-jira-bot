package modaltesting

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"

	"github.com/openshift/ci-tools/pkg/slack/modals"
	"github.com/openshift/ci-tools/pkg/testhelper"
	"github.com/vrutkovs/slack-jira-bot/pkg/jira"
	"github.com/vrutkovs/slack-jira-bot/pkg/slack/interactions"
)

type SubmissionTestCase struct {
	Name            string
	Filer           *jira.Fake
	Updater         *modals.FakeViewUpdater
	ExpectedPayload interface{}
	ExpectedError   bool
}

// ValidateSubmission validates a submission flow that files a Jira issue
func ValidateSubmission(t *testing.T, handler interactions.Handler, testCases ...SubmissionTestCase) {
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			var callback slack.InteractionCallback
			ReadCallbackFixture(t, &callback)
			out, err := handler.Handle(&callback, logrus.WithField("test", testCase.Name))
			select {
			case <-time.After(1 * time.Second):
				t.Fatalf("%s: timed out waiting for issue handler to be called", testCase.Name)
			case <-testCase.Updater.Called().Done():
				// all good, continue
			}
			testhelper.CompareWithFixture(t, out)
			if testCase.ExpectedError && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.Name)
			}
			if !testCase.ExpectedError && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.Name, err)
			}
			testCase.Filer.Validate(t)
			testCase.Updater.Validate(t)
		})
	}
}
