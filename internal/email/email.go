package email

import (
    "context"
    "fmt"
    "os"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/ses"
    "github.com/aws/aws-sdk-go-v2/service/ses/types"
)

type EmailService struct {
    client *ses.Client
    from   string
}

func NewEmailService() (*EmailService, error) {
    cfg, err := config.LoadDefaultConfig(context.TODO(),
        config.WithRegion(os.Getenv("AWS_REGION")),
    )
    if err != nil {
        return nil, fmt.Errorf("unable to load AWS config: %v", err)
    }

    return &EmailService{
        client: ses.NewFromConfig(cfg),
        from:   os.Getenv("SES_FROM_EMAIL"),
    }, nil
}

func (e *EmailService) SendEmail(to, subject, htmlBody, textBody string) error {
    input := &ses.SendEmailInput{
        Destination: &types.Destination{
            ToAddresses: []string{to},
        },
        Message: &types.Message{
            Body: &types.Body{
                Html: &types.Content{
                    Data: aws.String(htmlBody),
                    Charset: aws.String("UTF-8"),
                },
                Text: &types.Content{
                    Data: aws.String(textBody),
                    Charset: aws.String("UTF-8"),
                },
            },
            Subject: &types.Content{
                Data: aws.String(subject),
                Charset: aws.String("UTF-8"),
            },
        },
        Source: aws.String(e.from),
    }

    _, err := e.client.SendEmail(context.TODO(), input)
    if err != nil {
        return fmt.Errorf("failed to send email: %v", err)
    }

    return nil
}