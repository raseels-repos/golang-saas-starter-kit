package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/translate"
	"github.com/pkg/errors"
)

// Translator is an instance of AWS Translator service
type Translator struct {
	t *translate.Translate
}

// New returns a AWS Translator service which credentials are
// retrieved by the default AWS SDK credential chain (env, conf)
func New() (*Translator, error) {
	s, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating AWS session")
	}

	t := translate.New(s, aws.NewConfig().WithMaxRetries(3))

	return &Translator{
		t: t,
	}, nil
}

// T translate text from an origin locale to a set of target locales,
// the results are in the same order of target locales.
func (awsT *Translator) T(text string, sourceLocale string, targetLocale string) (string, error) {
	input := &translate.TextInput{
		SourceLanguageCode: aws.String(sourceLocale),
		Text:               &text,
	}
	input.TargetLanguageCode = aws.String(targetLocale)
	output, err := awsT.t.Text(input)
	if err != nil {
		return "", errors.Wrapf(err, "error while translating for locale %v", targetLocale)
	}

	return *output.TranslatedText, nil
}
