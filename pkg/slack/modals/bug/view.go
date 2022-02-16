package bug

import (
	"encoding/json"
	"text/template"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"

	"github.com/openshift/ci-tools/pkg/jira"
	"github.com/openshift/ci-tools/pkg/slack/interactions"
	"github.com/openshift/ci-tools/pkg/slack/modals"
)

// Identifier is the view identifier for this modal
const Identifier modals.Identifier = "bug"

const (
	blockIdCategory     = "category"
	blockIdOptional     = "optional"
	blockIdSymptom      = "symptom"
	blockIdExpected     = "expected"
	blockIdImpact       = "impact"
	blockIdReproduction = "reproduction"

	componentAI    = "Assisted Installer"
	componentUI    = "MGMT UI"
	componentSNO   = "SNO"
	componentOther = "Other"
)

// View is the modal view for submitting a new bug to Jira
func View() slack.ModalViewRequest {
	return slack.ModalViewRequest{
		Type:            slack.VTModal,
		PrivateMetadata: string(Identifier),
		Title:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "File a Bug"},
		Close:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Cancel"},
		Submit:          &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Submit"},
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			&slack.SectionBlock{
				Type: slack.MBTSection,
				Text: &slack.TextBlockObject{
					Type: slack.PlainTextType,
					Text: "Use this form to report a bug in the test platform or infrastructure.",
				},
			},
			&slack.InputBlock{
				Type:    slack.MBTInput,
				BlockID: modals.BlockIdTitle,
				Label:   &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Provide a title for this bug:"},
				Element: &slack.PlainTextInputBlockElement{Type: slack.METPlainTextInput},
			},
			&slack.InputBlock{
				Type:    slack.MBTInput,
				BlockID: blockIdCategory,
				Label:   &slack.TextBlockObject{Type: slack.PlainTextType, Text: "What component is affected?"},
				Element: &slack.SelectBlockElement{
					Type:        slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Select a category..."},
					Options: []*slack.OptionBlockObject{
						{Value: componentAI, Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: componentAI}},
						{Value: componentUI, Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: componentUI}},
						{Value: componentSNO, Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: componentSNO}},
						{Value: componentOther, Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: componentOther}},
					},
				},
			},
			&slack.InputBlock{
				Type:     slack.MBTInput,
				BlockID:  blockIdOptional,
				Optional: true,
				Label:    &slack.TextBlockObject{Type: slack.PlainTextType, Text: "If other, what best describes the bugged component?"},
				Element:  &slack.PlainTextInputBlockElement{Type: slack.METPlainTextInput},
			},
			&slack.DividerBlock{
				Type: slack.MBTDivider,
			},
			&slack.InputBlock{
				Type:    slack.MBTInput,
				BlockID: blockIdSymptom,
				Label:   &slack.TextBlockObject{Type: slack.PlainTextType, Text: "What incorrect behavior did you notice?"},
				Element: &slack.PlainTextInputBlockElement{Type: slack.METPlainTextInput, Multiline: true},
			},
			&slack.InputBlock{
				Type:    slack.MBTInput,
				BlockID: blockIdExpected,
				Label:   &slack.TextBlockObject{Type: slack.PlainTextType, Text: "What behavior did you expect instead?"},
				Element: &slack.PlainTextInputBlockElement{Type: slack.METPlainTextInput, Multiline: true},
			},
		}},
	}
}

// validateSubmissionHandler validates user input
func validateSubmissionHandler() interactions.PartialHandler {
	return interactions.PartialHandlerFunc(string(Identifier)+".validate", func(callback *slack.InteractionCallback, logger *logrus.Entry) (bool, []byte, error) {
		// if someone selected the "other" component, make sure they fill in the field
		otherSelected := false
		for _, action := range callback.View.State.Values[blockIdCategory] {
			if action.SelectedOption.Value == componentOther {
				otherSelected = true
			}
		}
		if otherSelected {
			for _, action := range callback.View.State.Values[blockIdOptional] {
				if action.Value == "" {
					logger.Debug("Detected invalid submission.")
					response, err := json.Marshal(&slack.ViewSubmissionResponse{
						ResponseAction: slack.RAErrors,
						Errors: map[string]string{
							blockIdOptional: "Provide a description of the other component.",
						},
					})
					if err != nil {
						logger.WithError(err).Error("Failed to marshal view submission response.")
						return true, nil, err
					}
					return true, response, nil
				}
			}
		}
		return false, nil, nil
	})
}

func issueParameters() modals.JiraIssueParameters {
	return modals.JiraIssueParameters{
		Id:        Identifier,
		IssueType: jira.IssueTypeBug,
		Template: template.Must(template.New(string(Identifier)).Parse(`h3. Symptomatic Behavior
{{ .` + blockIdSymptom + ` }}

h3. Expected Behavior
{{ .` + blockIdExpected + ` }}

h3. Category
{{ if eq .` + blockIdCategory + `_static_select "Other" }}Other: {{ .` + blockIdOptional + ` }}{{ else }}{{ .` + blockIdCategory + `_static_select }}{{ end }}`)),
		Fields: []string{modals.BlockIdTitle, blockIdCategory, blockIdOptional, blockIdSymptom, blockIdExpected},
	}
}

// processSubmissionHandler files a Jira issue for this form
func processSubmissionHandler(filer jira.IssueFiler, updater modals.ViewUpdater) interactions.PartialHandler {
	return interactions.PartialFromHandler(modals.ToJiraIssue(issueParameters(), filer, updater))
}

// Register creates a registration entry for the bug form
func Register(filer jira.IssueFiler, client *slack.Client) *modals.FlowWithViewAndFollowUps {
	return modals.ForView(Identifier, View()).WithFollowUps(map[slack.InteractionType]interactions.Handler{
		slack.InteractionTypeViewSubmission: interactions.MultiHandler(
			validateSubmissionHandler(),
			processSubmissionHandler(filer, client),
		),
	})
}
