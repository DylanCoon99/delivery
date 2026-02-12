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
    "net/http"
    "io"
    "encoding/csv"
    "encoding/json"
    "encoding/base64"
    "mime/multipart"
    "github.com/google/uuid"
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
    db          *sql.DB
    dbQueries   *queries.Queries
    sesClient   *ses.Client
    sesV2Client *sesv2.Client
)

// ErrEmailSuppressed indicates the email address is on a suppression list
var ErrEmailSuppressed = errors.New("email address is suppressed")

// ErrPermanentAPIFailure indicates a non-retryable API error (4xx responses)
var ErrPermanentAPIFailure = errors.New("permanent API failure")

type APIDeliveryConfig struct {
    URL        string            `json:"url"`
    Method     string            `json:"method,omitempty"`      // HTTP method, defaults to POST
    AuthType   string            `json:"auth_type,omitempty"`   // "api_key", "bearer", "basic", or empty
    APIKey     string            `json:"api_key,omitempty"`     // API key value
    AuthHeader string            `json:"auth_header,omitempty"` // Header name for API key (default: X-API-Key)
    BearerToken string           `json:"bearer_token,omitempty"`
    BasicUser  string            `json:"basic_user,omitempty"`
    BasicPass  string            `json:"basic_pass,omitempty"`
    Headers    map[string]string `json:"headers,omitempty"`     // Additional custom headers
    TimeoutSec int               `json:"timeout_sec,omitempty"` // Request timeout in seconds
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
    sesV2Client = sesv2.NewFromConfig(cfg)
    log.Println("SES clients initialized")

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

    // Extract campaign questions (for custom column headers)
    type QuestionInfo struct {
        ID           string
        QuestionText string
        DisplayOrder int
    }
    var questions []QuestionInfo

    // Debug: log the raw questions data from payload
    rawQuestions := payload["questions"]
    log.Printf("Raw questions from payload: %v (type: %T)", rawQuestions, rawQuestions)

    if questionsData, ok := payload["questions"].([]interface{}); ok {
        log.Printf("Found %d questions in payload", len(questionsData))
        for _, qInterface := range questionsData {
            if qMap, ok := qInterface.(map[string]interface{}); ok {
                q := QuestionInfo{}
                if id, ok := qMap["id"].(string); ok {
                    q.ID = id
                }
                if text, ok := qMap["question_text"].(string); ok {
                    q.QuestionText = text
                }
                if order, ok := qMap["display_order"].(float64); ok {
                    q.DisplayOrder = int(order)
                }
                log.Printf("Extracted question: ID=%s, Text=%s, Order=%d", q.ID, q.QuestionText, q.DisplayOrder)
                questions = append(questions, q)
            }
        }
    } else {
        log.Printf("Warning: Could not extract questions from payload - type assertion failed")
    }

    // Sort questions by display_order
    for i := 0; i < len(questions)-1; i++ {
        for j := i + 1; j < len(questions); j++ {
            if questions[j].DisplayOrder < questions[i].DisplayOrder {
                questions[i], questions[j] = questions[j], questions[i]
            }
        }
    }

    // Generate CSV
    csvBuffer := new(bytes.Buffer)
    writer := csv.NewWriter(csvBuffer)

    // Build CSV header with base columns plus question text columns
    header := []string{"Email", "First Name", "Phone Number",
        "Company Name", "Employee Size", "Publisher Name",
        "LinkedIn Company", "LinkedIn Contact", "Downloaded Asset Name",
        "State", "Region", "Address", "Industry", "IP Address", "Date/Time Stamp"}

    // Add question text as column headers
    for _, q := range questions {
        header = append(header, q.QuestionText)
    }
    log.Printf("CSV header built with %d total columns (%d base + %d question columns)", len(header), 15, len(questions))
    log.Printf("Header columns: %v", header)
    writer.Write(header)

    // Write lead data
    firstLead := true
    for _, leadInterface := range leadsData {

        lead := leadInterface.(map[string]interface{})

        // Debug: log the first lead's CustomAnswers to help diagnose issues
        if firstLead {
            if ca, exists := lead["CustomAnswers"]; exists {
                log.Printf("First lead CustomAnswers: %v (type: %T)", ca, ca)
            } else {
                log.Printf("First lead has no CustomAnswers field")
            }
            firstLead = false
        }
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

        // Helper function to extract IP address (stored as IPNet structure)
        extractIPAddress := func(field interface{}) string {
            if field == nil {
                return ""
            }
            if fieldMap, ok := field.(map[string]interface{}); ok {
                // Check Valid flag
                if valid, exists := fieldMap["Valid"]; exists {
                    if validBool, ok := valid.(bool); ok && !validBool {
                        return ""
                    }
                }
                // Extract IP from IPNet structure: { IPNet: { IP: "x.x.x.x" }, Valid: true }
                if ipNet, exists := fieldMap["IPNet"]; exists && ipNet != nil {
                    if ipNetMap, ok := ipNet.(map[string]interface{}); ok {
                        if ip, exists := ipNetMap["IP"]; exists && ip != nil {
                            if ipStr, ok := ip.(string); ok {
                                return ipStr
                            }
                        }
                    }
                }
            }
            return ""
        }

        // Helper function to extract timestamp (stored as Time structure)
        extractTimestamp := func(field interface{}) string {
            if field == nil {
                return ""
            }
            if fieldMap, ok := field.(map[string]interface{}); ok {
                // Check Valid flag
                if valid, exists := fieldMap["Valid"]; exists {
                    if validBool, ok := valid.(bool); ok && !validBool {
                        return ""
                    }
                }
                // Extract Time value
                if timeVal, exists := fieldMap["Time"]; exists && timeVal != nil {
                    if timeStr, ok := timeVal.(string); ok {
                        return timeStr
                    }
                }
            }
            return ""
        }

        row := []string{
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
            extractIPAddress(lead["IpAddress"]),
            extractTimestamp(lead["CapturedAt"]),
        }

        // Extract custom answers for each question
        var customAnswers map[string]interface{}
        if ca, ok := lead["CustomAnswers"]; ok && ca != nil {
            // Handle JSONB from database - may be wrapped in pqtype.NullRawMessage structure
            if caMap, ok := ca.(map[string]interface{}); ok {
                // Check if it's a pqtype.NullRawMessage structure with RawMessage and Valid fields
                if rawMsg, hasRaw := caMap["RawMessage"]; hasRaw {
                    if valid, hasValid := caMap["Valid"]; hasValid {
                        if validBool, ok := valid.(bool); ok && validBool {
                            // RawMessage is already deserialized as a map
                            if rawMap, ok := rawMsg.(map[string]interface{}); ok {
                                customAnswers = rawMap
                            }
                        }
                    }
                } else {
                    // Direct map without RawMessage wrapper
                    customAnswers = caMap
                }
            }
        }

        // Add custom answer values in question order
        for _, q := range questions {
            answer := ""
            if customAnswers != nil {
                if val, exists := customAnswers[q.ID]; exists && val != nil {
                    answer = extractString(val)
                } else {
                    log.Printf("DEBUG: No match for question ID %s in CustomAnswers (keys: %v)", q.ID, customAnswers)
                }
            }
            row = append(row, answer)
        }

        writer.Write(row)
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
    case "api":
        var cfg APIDeliveryConfig
        if err := json.Unmarshal(method.Config, &cfg); err != nil {
            return fmt.Errorf("invalid api config json: %w", err)
        }
        if cfg.URL == "" {
            return fmt.Errorf("api method missing url config")
        }
        deliveryErr = deliverAPI(ctx, job, &method, cfg, csvBuffer.Bytes(), fmt.Sprintf("leads_%s.csv", time.Now().Format("20060102_150405")))
    default:
        return fmt.Errorf("unknown delivery method type: %s", method.MethodType.String)
    }


    // Update job status
    // Determine final status
    var status string
    lastErr := sql.NullString{Valid: false}

    if deliveryErr != nil {
        // Check if this is a permanent failure (suppressed email or 4xx API error - no retry)
        if errors.Is(deliveryErr, ErrEmailSuppressed) || errors.Is(deliveryErr, ErrPermanentAPIFailure) {
            status = "failed"
            log.Printf("Job %s permanently failed: %v", job.ID, deliveryErr)
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
        status = "success"
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

    // Update delivery status if delivery_id is set
    if job.DeliveryID.Valid {
        if err := q.UpdateDeliveryStatus(ctx, queries.UpdateDeliveryStatusParams{
            ID:       job.DeliveryID.UUID,
            TenantID: job.TenantID,
            Status:   utils.SqlNullString(status),
        }); err != nil {
            log.Printf("Failed to update delivery status: %v", err)
        }
    }

    // If delivery was successful, increment the campaign's delivered lead count
    if status == "success" {
        // Extract campaign_id and total_leads from payload
        if campaignIDStr, ok := payload["campaign_id"].(string); ok && campaignIDStr != "" {
            campaignID, parseErr := uuid.Parse(campaignIDStr)
            if parseErr == nil {
                // Get total leads count from payload
                totalLeads := int32(0)
                if leadsData, ok := payload["leads"].([]interface{}); ok {
                    totalLeads = int32(len(leadsData))
                } else if totalLeadsVal, ok := payload["total_leads"].(float64); ok {
                    totalLeads = int32(totalLeadsVal)
                }

                if totalLeads > 0 {
                    if err := q.IncrementCampaignDeliveredCount(ctx, queries.IncrementCampaignDeliveredCountParams{
                        ID:                 campaignID,
                        TenantID:           job.TenantID,
                        DeliveredLeadCount: totalLeads,
                    }); err != nil {
                        log.Printf("Failed to increment campaign delivered count: %v", err)
                    } else {
                        log.Printf("Successfully incremented campaign %s delivered count by %d", campaignID, totalLeads)
                    }
                }
            }
        }
    }


    // Add history entry
    historyStatus := status
    if deliveryErr != nil && status == "pending" {
        historyStatus = "retry_scheduled"
    } else if errors.Is(deliveryErr, ErrEmailSuppressed) {
        historyStatus = "suppressed"
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



// checkSESSuppressionList checks if the email is on AWS SES account-level suppression list
func checkSESSuppressionList(ctx context.Context, email string) (bool, string, error) {
    result, err := sesV2Client.GetSuppressedDestination(ctx, &sesv2.GetSuppressedDestinationInput{
        EmailAddress: aws.String(email),
    })
    if err != nil {
        // NotFound error means email is not suppressed - this is expected
        var notFound *sesv2types.NotFoundException
        if errors.As(err, &notFound) {
            return false, "", nil
        }
        return false, "", fmt.Errorf("failed to check SES suppression list: %w", err)
    }

    if result.SuppressedDestination != nil {
        reason := string(result.SuppressedDestination.Reason)
        return true, fmt.Sprintf("ses_%s", reason), nil
    }

    return false, "", nil
}

// isEmailSuppressed checks if the email is on the SES suppression list
func isEmailSuppressed(ctx context.Context, email string) (bool, string, error) {
    return checkSESSuppressionList(ctx, email)
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

// deliverAPI sends leads as a CSV file to a configured HTTP endpoint
func deliverAPI(ctx context.Context, job *queries.DeliveryJob, method *queries.DeliveryMethod, cfg APIDeliveryConfig, csvData []byte, filename string) error {
    // Create multipart form with CSV file
    var requestBody bytes.Buffer
    multipartWriter := multipart.NewWriter(&requestBody)

    // Add metadata fields
    multipartWriter.WriteField("job_id", job.ID.String())
    multipartWriter.WriteField("buyer_id", job.BuyerID.String())
    multipartWriter.WriteField("tenant_id", job.TenantID.String())

    // Add CSV file
    fileWriter, err := multipartWriter.CreateFormFile("file", filename)
    if err != nil {
        return fmt.Errorf("failed to create form file: %w", err)
    }
    if _, err := fileWriter.Write(csvData); err != nil {
        return fmt.Errorf("failed to write csv data: %w", err)
    }

    // Close the multipart writer to finalize the boundary
    if err := multipartWriter.Close(); err != nil {
        return fmt.Errorf("failed to close multipart writer: %w", err)
    }

    // Determine HTTP method (default to POST)
    httpMethod := cfg.Method
    if httpMethod == "" {
        httpMethod = "POST"
    }

    // Create the request
    req, err := http.NewRequestWithContext(ctx, httpMethod, cfg.URL, &requestBody)
    if err != nil {
        return fmt.Errorf("failed to create request: %w", err)
    }

    // Set content type with multipart boundary
    req.Header.Set("Content-Type", multipartWriter.FormDataContentType())

    // Apply authentication based on auth_type
    switch cfg.AuthType {
    case "api_key":
        headerName := cfg.AuthHeader
        if headerName == "" {
            headerName = "X-API-Key"
        }
        req.Header.Set(headerName, cfg.APIKey)
    case "bearer":
        req.Header.Set("Authorization", "Bearer "+cfg.BearerToken)
    case "basic":
        req.SetBasicAuth(cfg.BasicUser, cfg.BasicPass)
    }

    // Apply any custom headers
    for key, value := range cfg.Headers {
        req.Header.Set(key, value)
    }

    // Configure timeout
    timeout := 30 * time.Second
    if cfg.TimeoutSec > 0 {
        timeout = time.Duration(cfg.TimeoutSec) * time.Second
    }

    client := &http.Client{
        Timeout: timeout,
    }

    log.Printf("Sending API request to %s with CSV file %s (%d bytes)", cfg.URL, filename, len(csvData))

    // Execute the request
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("api request failed: %w", err)
    }
    defer resp.Body.Close()

    // Read response body for error reporting
    respBody, _ := io.ReadAll(resp.Body)

    // Handle response status codes
    if resp.StatusCode >= 200 && resp.StatusCode < 300 {
        log.Printf("API delivery successful: status %d", resp.StatusCode)
        return nil
    }

    // 4xx errors are permanent failures (bad request, unauthorized, forbidden, not found)
    if resp.StatusCode >= 400 && resp.StatusCode < 500 {
        log.Printf("API delivery permanently failed: status %d, body: %s", resp.StatusCode, string(respBody))
        return fmt.Errorf("%w: status %d - %s", ErrPermanentAPIFailure, resp.StatusCode, string(respBody))
    }

    // 5xx errors are retryable
    log.Printf("API delivery failed (retryable): status %d, body: %s", resp.StatusCode, string(respBody))
    return fmt.Errorf("api responded with status %d: %s", resp.StatusCode, string(respBody))
}

func main() {
    lambda.Start(handler)
}
