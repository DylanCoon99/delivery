package main

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "log"
    "os"

    "net/url"
    "time"
    "bytes"
    //"net/http"
    "encoding/csv"
    "encoding/json"
    "encoding/base64"
    //"github.com/google/uuid"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/ses/types"
    "github.com/aws/aws-sdk-go-v2/service/ses"
    "github.com/aws/aws-sdk-go-v2/service/sesv2"
    sesv2types "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
    _ "github.com/jackc/pgx/v5/stdlib"
    //"github.com/DylanCoon99/delivery/cmd/types"
    "github.com/DylanCoon99/delivery/internal/utils"
    "github.com/DylanCoon99/delivery/internal/database/queries"

)

var (
    db         *sql.DB          // Add this global variable
    dbQueries  *queries.Queries
    sesClient  *ses.Client
    sesv2Client *sesv2.Client
)

// ErrEmailSuppressed indicates the email address is on a suppression list
var ErrEmailSuppressed = errors.New("email address is suppressed")

type WebhookDeliveryConfig struct {
    URL        string `json:"url"`
    APIKey     string `json:"api_key,omitempty"`
    AuthHeader string `json:"auth_header,omitempty"`
}

/*
func init() {

    // Initialize AWS SES client
    cfg, err := config.LoadDefaultConfig(context.Background())
    if err != nil {
        log.Fatalf("Failed to load AWS config: %v", err)
    }
    
    sesClient = ses.NewFromConfig(cfg)
    log.Println("SES client initialized")

    for i := 0; i < 5; i++ {
        log.Println("TESTING")
    }

}
*/



func init() {
    log.Println("Initializing Lambda function...")
    
    // Get database credentials from Secrets Manager
    secret, err := utils.GetDBSecret()
    if err != nil {
        log.Fatalf("Failed to get database secret: %v", err)
    }
    log.Println("Retrieved database credentials")
    
    // Get database connection details from environment
    host := os.Getenv("HOST")
    port := os.Getenv("PORT")
    dbName := os.Getenv("DB_NAME")
    
    // Validate environment variables
    if host == "" {
        log.Fatal("HOST environment variable not set")
    }
    if port == "" {
        port = "5432" // default PostgreSQL port
    }
    if dbName == "" {
        log.Fatal("DB_NAME environment variable not set")
    }
    
    log.Printf("Connecting to: %s:%s/%s", host, port, dbName)
    
    // URL encode password to handle special characters
    encodedPassword := url.QueryEscape(secret.Password)
    
    // Build connection string
    dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=require",
        secret.Username,
        encodedPassword,
        host,
        port,
        dbName,
    )
    
    // Open database connection (use pgx driver, store in GLOBAL db variable)
    db, err = sql.Open("pgx", dsn)  //  No := here, assigns to global db
    if err != nil {
        log.Fatalf("Failed to open database: %v", err)
    }
    
    // Test the connection
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    if err := db.PingContext(ctx); err != nil {
        log.Fatalf("Failed to ping database: %v", err)
    }
    log.Println("Database connection established")
    
    // Configure connection pool
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)
    
    // Create queries object
    dbQueries = queries.New(db)
    log.Println("Database queries initialized")
    
    // Initialize AWS SES client
    cfg, err := config.LoadDefaultConfig(context.Background())
    if err != nil {
        log.Fatalf("Failed to load AWS config: %v", err)
    }
    
    sesClient = ses.NewFromConfig(cfg)
    sesv2Client = sesv2.NewFromConfig(cfg)
    log.Println("SES client initialized")

    log.Println("Lambda initialization complete")
}


// Main Lambda entrypoint
func handler(ctx context.Context) error {

    limit := int32(50) // default
    offset := int32(0) // default

    // TODO: iterate through tenants — or add "global pending jobs" endpoint
    tenants, err := dbQueries.ListTenants(ctx, queries.ListTenantsParams{
        Limit:  limit,
        Offset: offset,
    })
    if err != nil {
        return fmt.Errorf("failed to fetch tenants: %w", err)
    } else {
        log.Printf("Got tenants: %v", tenants)
    }

    if err := processJobs(ctx, dbQueries); err != nil {
        log.Printf("Error processing jobs %v",  err)
    }

    return nil
}


func processJobs(ctx context.Context, q *queries.Queries) error {
    
    pending, err := q.GetDueJobs(ctx)

    if err != nil {
        return err
    } else {
        log.Printf("Got Pending Jobs: %v", pending)
    }

    for _, job := range pending {
        if err := processJob(ctx, q, &job); err != nil {
            log.Printf("job %s failed: %v", job.ID, err)
        }
    }

    return nil
}

func processJob(ctx context.Context, q *queries.Queries, job *queries.DeliveryJob) error {
    
    const maxRetries = 3
    
    // Increment attempts
    if err := q.IncrementDeliveryJobAttempts(ctx, queries.IncrementDeliveryJobAttemptsParams{
        ID:       job.ID,
        TenantID: job.TenantID,
    }); err != nil {
        log.Printf("Failed to increment attempts: %v", err)
    }


    // Parse payload
    var payload map[string]interface{}
    if err := json.Unmarshal(job.Payload, &payload); err != nil {
        return fmt.Errorf("failed to parse payload: %w", err)
    }

    // Extract leads
    leadsData, ok := payload["leads"].([]interface{})
    if !ok {
        return fmt.Errorf("invalid leads data in payload")
    }

    log.Printf("Leads: %v", leadsData)

    // Generate CSV
    csvBuffer := new(bytes.Buffer)
    writer := csv.NewWriter(csvBuffer)
    
    // Write CSV header
    writer.Write([]string{"Email", "First Name", "Phone Number", 
        "Company Name", "Employee Size", "Publisher Name", 
        "LinkedIn Company", "LinkedIn Contact", "Downloaded Asset Name",
        "State", "Region", "Address", "Industry", "IP Address"}) // Add your columns
    
    // Write lead data
    for _, leadInterface := range leadsData {

        lead := leadInterface.(map[string]interface{})
        // Helper function to safely extract string values
        extractString := func(field interface{}) string {
            if field == nil {
                return ""
            }
            // Handle nested map structure (e.g., map[String:value Valid:true])
            if fieldMap, ok := field.(map[string]interface{}); ok {
                if strVal, exists := fieldMap["String"]; exists && strVal != nil {
                    if str, ok := strVal.(string); ok {
                        return str
                    }
                }
            }
            // Handle direct string
            if str, ok := field.(string); ok {
                return str
            }
            return ""
        }
        
        writer.Write([]string{
            extractString(lead["EmailHash"]),
            extractString(lead["FullName"]),
            extractString(lead["PhoneHash"]),
            extractString(lead["CompanyName"]),
            extractString(lead["EmployeeSize"]),
            extractString(lead["PublisherName"]),
            extractString(lead["LinkedinCompany"]),
            extractString(lead["LinkedinContact"]),
            extractString(lead["DownloadedAssetName"]),
            extractString(lead["State"]),
            extractString(lead["Region"]),
            extractString(lead["Address"]),
            extractString(lead["Industry"]),
            extractString(lead["IpAddress"]),
        })
    }
    writer.Flush()


    method, err := q.GetDeliveryMethod(ctx, queries.GetDeliveryMethodParams{
        ID:       job.DeliveryMethodID,
        TenantID: job.TenantID,
    })
    
    if err != nil {
        return fmt.Errorf("failed to fetch delivery method: %w", err)
    }


    // Execute delivery
    var deliveryErr error

    switch method.MethodType.String {
    case "email":
        deliveryErr = deliverEmail(ctx, job, &method, csvBuffer.Bytes(), fmt.Sprintf("leads_%s.csv", time.Now().Format("20060102_150405")))
    /*
    case "webhook":
        var cfg WebhookDeliveryConfig
        if err := json.Unmarshal(method.Config, &cfg); err != nil {
            return fmt.Errorf("invalid webhook config json: %w", err)
        }
        if cfg.URL == "" {
            return fmt.Errorf("webhook method missing url config")
        }
        deliveryErr = deliverWebhook(ctx, job, &method, cfg)
    */
    default:
        return fmt.Errorf("unknown delivery method type: %s", method.MethodType.String)
    }


    // Update job status
    // Determine final status
    var status string
    lastErr := sql.NullString{Valid: false}

    if deliveryErr != nil {
        // Check if this is a permanent failure (suppressed email - no retry)
        if errors.Is(deliveryErr, ErrEmailSuppressed) {
            status = "failed"
            log.Printf("Job %s permanently failed - email suppressed: %v", job.ID, deliveryErr)
        } else if job.Attempts+1 < maxRetries {
            // Retryable error - keep as pending
            status = "pending"
            log.Printf("Job %s failed (attempt %d/%d), will retry: %v", job.ID, job.Attempts+1, maxRetries, deliveryErr)
        } else {
            status = "failed" // Max retries reached
            log.Printf("Job %s failed after %d attempts: %v", job.ID, job.Attempts+1, deliveryErr)
        }
        lastErr = sql.NullString{String: deliveryErr.Error(), Valid: true}
    } else {
        status = "completed"
        log.Printf("Job %s completed successfully on attempt %d", job.ID, job.Attempts+1)
    }


    // Update job status
    if _, err := q.UpdateDeliveryJobStatus(ctx, queries.UpdateDeliveryJobStatusParams{
        ID:        job.ID,
        Status:    status,
        TenantID:  job.TenantID,
        LastError: lastErr,
    }); err != nil {
        return fmt.Errorf("failed to update job status: %w", err)
    }

    // Add history entry
    historyStatus := status
    if deliveryErr != nil && status == "pending" {
        historyStatus = "retry_scheduled"
    }

    // Add history
    if _, err := q.CreateDeliveryHistory(ctx, queries.CreateDeliveryHistoryParams{
        TenantID:          job.TenantID,
        JobID:             utils.NullUUID(job.ID),
        BuyerID:           utils.NullUUID(job.BuyerID),
        DeliveryMethodID:  utils.NullUUID(job.DeliveryMethodID),
        Status:            utils.SqlNullString(historyStatus),
        ErrorMessage:      utils.SqlNullString(lastErr.String),
    }); err != nil {
        return fmt.Errorf("failed to insert history: %w", err)
    }

    return deliveryErr
}



// checkSESSuppressionList checks if an email is on AWS SES account-level suppression list
func checkSESSuppressionList(ctx context.Context, email string) (bool, string, error) {
    result, err := sesv2Client.GetSuppressedDestination(ctx, &sesv2.GetSuppressedDestinationInput{
        EmailAddress: aws.String(email),
    })
    if err != nil {
        // NotFound error means email is not suppressed
        var notFound *sesv2types.NotFoundException
        if errors.As(err, &notFound) {
            return false, "", nil
        }
        return false, "", fmt.Errorf("failed to check SES suppression list: %w", err)
    }

    reason := string(result.SuppressedDestination.Reason)
    log.Printf("Email %s is on SES suppression list, reason: %s", email, reason)
    return true, reason, nil
}

// checkLocalSuppressionHistory checks the local database for bounce/complaint history
func checkLocalSuppressionHistory(ctx context.Context, email string) (bool, string, error) {
    // Check for hard bounces (permanent failures)
    bounceCount, err := dbQueries.GetBounceCountByEmail(ctx, email)
    if err != nil {
        return false, "", fmt.Errorf("failed to check bounce count: %w", err)
    }
    if bounceCount > 0 {
        return true, "previous_bounce", nil
    }

    // Check for complaints (spam reports)
    complaintCount, err := dbQueries.GetComplaintCountByEmail(ctx, email)
    if err != nil {
        return false, "", fmt.Errorf("failed to check complaint count: %w", err)
    }
    if complaintCount > 0 {
        return true, "previous_complaint", nil
    }

    return false, "", nil
}

// isEmailSuppressed checks both AWS SES and local database for suppression
func isEmailSuppressed(ctx context.Context, email string) (bool, string, error) {
    // Check AWS SES suppression list first
    suppressed, reason, err := checkSESSuppressionList(ctx, email)
    if err != nil {
        log.Printf("Warning: failed to check SES suppression list: %v", err)
        // Continue to check local database even if SES check fails
    }
    if suppressed {
        return true, fmt.Sprintf("ses_suppression: %s", reason), nil
    }

    // Check local bounce/complaint history
    suppressed, reason, err = checkLocalSuppressionHistory(ctx, email)
    if err != nil {
        return false, "", err
    }
    if suppressed {
        return true, reason, nil
    }

    return false, "", nil
}

// SES email sender with retry-friendly error handling
func deliverEmail(ctx context.Context, job *queries.DeliveryJob, method *queries.DeliveryMethod, csvData []byte, filename string) error {
    const SenderAddress = "notifications@lead-ship.app"
    

    // Parse payload to get lead count and recipient email
    var payload map[string]interface{}
    leadCount := 0
    recipientEmail := ""
    
    if err := json.Unmarshal(job.Payload, &payload); err != nil {
        return fmt.Errorf("failed to parse job payload: %w", err)
    }
    
    // Extract lead count
    if leads, ok := payload["leads"].([]interface{}); ok {
        leadCount = len(leads)
    }
    
    // Extract recipient email from payload
    if emailVal, exists := payload["recipient_email"]; exists && emailVal != nil {
        if email, ok := emailVal.(string); ok && email != "" {
            recipientEmail = email
        }
    }
    
    if recipientEmail == "" {
        return fmt.Errorf("recipient email not found in job payload")
    }

    // Check if email is suppressed before attempting to send
    suppressed, reason, err := isEmailSuppressed(ctx, recipientEmail)
    if err != nil {
        log.Printf("Warning: suppression check failed for %s: %v", recipientEmail, err)
        // Continue with send attempt if suppression check fails
    }
    if suppressed {
        log.Printf("Skipping delivery to suppressed email %s, reason: %s", recipientEmail, reason)
        return fmt.Errorf("%w: %s", ErrEmailSuppressed, reason)
    }

    // Create email body with attachment
    subject := "New Lead Delivery"
    bodyText := fmt.Sprintf("Please find attached %d leads in CSV format.", leadCount)
    
    // Build MIME message with attachment
    boundary := "boundary123"
    
    rawMessage := fmt.Sprintf(`From: %s
To: %s
Subject: %s
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="%s"

--%s
Content-Type: text/plain; charset=UTF-8

%s

--%s
Content-Type: text/csv; name="%s"
Content-Disposition: attachment; filename="%s"
Content-Transfer-Encoding: base64

%s
--%s--`,
        SenderAddress,
        recipientEmail,
        subject,
        boundary,
        boundary,
        bodyText,
        boundary,
        filename,
        filename,
        base64.StdEncoding.EncodeToString(csvData),
        boundary,
    )
    
    log.Printf("Attempting to send email to %s with %d leads", recipientEmail, leadCount)
    
    _, err = sesClient.SendRawEmail(ctx, &ses.SendRawEmailInput{
        RawMessage: &types.RawMessage{
            Data: []byte(rawMessage),
        },
        Source: aws.String(SenderAddress),
    })
    
    if err != nil {
        log.Printf("Email delivery failed: %v", err)
        return fmt.Errorf("email delivery failed: %w", err)
    }
    
    log.Printf("Email successfully sent to %s", recipientEmail)
    return nil
}
/*
func deliverWebhook(ctx context.Context, job *queries.DeliveryJob, method *queries.DeliveryMethod, cfg WebhookDeliveryConfig) error {
    payload := map[string]any{
        "job_id":       job.ID.String(),
        "buyer_id":     job.BuyerID.String(),
        "tenant_id":    job.TenantID.String(),
        "lead_payload": job.Payload,
    }

    url := cfg.URL

    body, _ := json.Marshal(payload)

    req, err := http.NewRequest("POST", url, bytes.NewReader(body))
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 300 {
        return fmt.Errorf("webhook responded with status %d", resp.StatusCode)
    }

    return nil
}
*/

func main() {
    lambda.Start(handler)
}
