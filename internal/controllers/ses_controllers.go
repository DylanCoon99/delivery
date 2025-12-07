package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"github.com/gin-gonic/gin"
	"github.com/sqlc-dev/pqtype"
	"github.com/robbiet480/go.sns"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/database/queries"
)



// SNS Message structure
type SNSMessage struct {
	Type             string `json:"Type"`
	MessageId        string `json:"MessageId"`
	Token            string `json:"Token"`
	TopicArn         string `json:"TopicArn"`
	Subject          string `json:"Subject"`
	Message          string `json:"Message"`
	Timestamp        string `json:"Timestamp"`
	SignatureVersion string `json:"SignatureVersion"`
	Signature        string `json:"Signature"`
	SigningCertURL   string `json:"SigningCertURL"`
	SubscribeURL     string `json:"SubscribeURL"`
	UnsubscribeURL   string `json:"UnsubscribeURL"`
}

// SES Notification structure
type SESNotification struct {
	NotificationType string          `json:"notificationType"`
	Bounce           *BounceDetails  `json:"bounce,omitempty"`
	Complaint        *ComplaintDetails `json:"complaint,omitempty"`
	Delivery         *DeliveryDetails  `json:"delivery,omitempty"`
}

type BounceDetails struct {
	BounceType        string              `json:"bounceType"`
	BounceSubType     string              `json:"bounceSubType"`
	BouncedRecipients []BouncedRecipient  `json:"bouncedRecipients"`
	Timestamp         string              `json:"timestamp"`
	FeedbackId        string              `json:"feedbackId"`
}

type BouncedRecipient struct {
	EmailAddress   string `json:"emailAddress"`
	Action         string `json:"action"`
	Status         string `json:"status"`
	DiagnosticCode string `json:"diagnosticCode"`
}

type ComplaintDetails struct {
	ComplainedRecipients []ComplainedRecipient `json:"complainedRecipients"`
	Timestamp            string                `json:"timestamp"`
	FeedbackId           string                `json:"feedbackId"`
	ComplaintFeedbackType string               `json:"complaintFeedbackType"`
}

type ComplainedRecipient struct {
	EmailAddress string `json:"emailAddress"`
}

type DeliveryDetails struct {
	Timestamp            string   `json:"timestamp"`
	ProcessingTimeMillis int      `json:"processingTimeMillis"`
	Recipients           []string `json:"recipients"`
}



func verifySNSSignature(msg SNSMessage) error {
	// Convert to the library's payload format
	payload := &sns.Payload{
		Message:          msg.Message,
		MessageId:        msg.MessageId,
		Signature:        msg.Signature,
		SignatureVersion: msg.SignatureVersion,
		SigningCertURL:   msg.SigningCertURL,
		Subject:          msg.Subject,
		Timestamp:        msg.Timestamp,
		Token:            msg.Token,
		TopicArn:         msg.TopicArn,
		Type:             msg.Type,
		SubscribeURL:     msg.SubscribeURL,
		UnsubscribeURL:   msg.UnsubscribeURL,
	}
	
	return payload.VerifyPayload()
}



// Handle bounce notifications
func (cfg *ApiConfig) handleBounce(bounce *BounceDetails) {
	//ctx := context.Background()

	log.Printf("Bounce type: %s, subtype: %s", bounce.BounceType, bounce.BounceSubType)
	
	for _, recipient := range bounce.BouncedRecipients {
		log.Printf("Bounced email: %s", recipient.EmailAddress)
		log.Printf("Diagnostic code: %s", recipient.DiagnosticCode)
		
		// Handle permanent bounces (hard bounces)
		if bounce.BounceType == "Permanent" {
			log.Printf("Hard bounce detected for %s - marking as permanently bounced", recipient.EmailAddress)
			// TODO: Mark email as bounced in your database
			// markEmailAsBounced(recipient.EmailAddress, "permanent")
		}
		
		// Handle transient bounces (soft bounces)
		if bounce.BounceType == "Transient" {
			log.Printf("Soft bounce detected for %s", recipient.EmailAddress)
			// TODO: Increment soft bounce counter
			// incrementSoftBounceCount(recipient.EmailAddress)
		}
	}
}

// Handle complaint notifications
func (cfg *ApiConfig) handleComplaint(complaint *ComplaintDetails) {
	ctx := context.Background()

	log.Printf("Complaint feedback type: %s", complaint.ComplaintFeedbackType)
	
	for _, recipient := range complaint.ComplainedRecipients {
		log.Printf("Complaint from: %s", recipient.EmailAddress)
		
		// Serialize the entire complaint details as raw JSON
		rawJSON, err := json.Marshal(complaint)
		if err != nil {
			log.Printf("Error marshaling complaint data: %v", err)
			continue
		}

		// Create email event record
		event, err := cfg.DBQueries.CreateEmailEvent(ctx, queries.CreateEmailEventParams{
			Email:     recipient.EmailAddress,
			EventType: "complaint",
			EventSubtype: sql.NullString{
				String: complaint.ComplaintFeedbackType,
				Valid:  complaint.ComplaintFeedbackType != "",
			},
			Reason: sql.NullString{
				String: "spam_complaint",
				Valid:  true,
			},
			DiagnosticCode: sql.NullString{
				String: "",
				Valid:  false,
			},
			FeedbackID: sql.NullString{
				String: complaint.FeedbackId,
				Valid:  complaint.FeedbackId != "",
			},
			MessageID: sql.NullString{
				String: "",
				Valid:  false,
			},
			RawData: pqtype.NullRawMessage{
				RawMessage: rawJSON,
				Valid: true,
			},
		})

		if err != nil {
			log.Printf("Error creating complaint event for %s: %v", recipient.EmailAddress, err)
			continue
		}

		log.Printf("Created complaint event ID %d for %s", event.ID, recipient.EmailAddress)

	}
}

// Handle delivery notifications
func (cfg *ApiConfig) handleDelivery(delivery *DeliveryDetails) {
	ctx := context.Background()

	log.Printf("Email delivered successfully at %s", delivery.Timestamp)

	for _, recipient := range delivery.Recipients {
		log.Printf("Delivered to: %s", recipient)
		// Serialize the entire delivery details as raw JSON
		rawJSON, err := json.Marshal(delivery)
		if err != nil {
			log.Printf("Error marshaling delivery data: %v", err)
			continue
		}

		// Create email event record
		event, err := cfg.DBQueries.CreateEmailEvent(ctx, queries.CreateEmailEventParams{
			Email:     recipient,
			EventType: "delivery",
			EventSubtype: sql.NullString{
				String: "success",
				Valid:  true,
			},
			Reason: sql.NullString{
				String: "",
				Valid:  false,
			},
			DiagnosticCode: sql.NullString{
				String: "",
				Valid:  false,
			},
			FeedbackID: sql.NullString{
				String: "",
				Valid:  false,
			},
			MessageID: sql.NullString{
				String: "", // Can be added if available in your delivery structure
				Valid:  false,
			},
			RawData: pqtype.NullRawMessage{
				RawMessage: rawJSON,
				Valid: true,
			},
		})

		if err != nil {
			log.Printf("Error creating delivery event for %s: %v", recipient, err)
			continue
		}

		log.Printf("Created delivery event ID %d for %s", event.ID, recipient)
	}
}

func (cfg *ApiConfig) SESWebhookHandler(c *gin.Context) {
	// Parse the SNS message
	var snsMsg SNSMessage
	if err := c.ShouldBindJSON(&snsMsg); err != nil {
		log.Printf("Error decoding SNS message: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bad request"})
		return
	}

	// Verify the signature
	err := verifySNSSignature(snsMsg)
	if err != nil {
		log.Printf("Invalid SNS signature: %v", err)
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}

	// Handle subscription confirmation
	if snsMsg.Type == "SubscriptionConfirmation" {
		log.Printf("Subscription confirmation URL: %s", snsMsg.SubscribeURL)
		
		// Auto-confirm the subscription
		resp, err := http.Get(snsMsg.SubscribeURL)
		if err != nil {
			log.Printf("Error confirming subscription: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}
		defer resp.Body.Close()
		
		log.Println("Subscription confirmed successfully")
		c.String(http.StatusOK, "Subscription confirmed")
		return
	}

	// Handle notification
	if snsMsg.Type == "Notification" {
		var sesNotification SESNotification
		if err := json.Unmarshal([]byte(snsMsg.Message), &sesNotification); err != nil {
			log.Printf("Error parsing SES notification: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Bad request"})
			return
		}

		// Route to appropriate handler
		switch sesNotification.NotificationType {
		case "Bounce":
			if sesNotification.Bounce != nil {
				cfg.handleBounce(sesNotification.Bounce)
			}
		case "Complaint":
			if sesNotification.Complaint != nil {
				cfg.handleComplaint(sesNotification.Complaint)
			}
		case "Delivery":
			if sesNotification.Delivery != nil {
				cfg.handleDelivery(sesNotification.Delivery)
			}
		default:
			log.Printf("Unknown notification type: %s", sesNotification.NotificationType)
		}
	}

	c.String(http.StatusOK, "OK")
}