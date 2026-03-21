package main

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "log"
    "os"
    "strings"
    "sync"

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

    // Credential caching
    credentialsMu      sync.Mutex
    credentialsLastRefresh time.Time
    credentialsTTL     = 15 * time.Minute // Refresh credentials every 15 minutes
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



// connectDB establishes database connection with current credentials from Secrets Manager
func connectDB() error {
    credentialsMu.Lock()
    defer credentialsMu.Unlock()

    // Get database credentials from Secrets Manager
    secret, err := utils.GetDBSecret()
    if err != nil {
        return fmt.Errorf("failed to get database secret: %w", err)
    }
    log.Println("Retrieved database credentials from Secrets Manager")

    // Get database connection details from environment
    host := os.Getenv("HOST")
    port := os.Getenv("PORT")
    dbName := os.Getenv("DB_NAME")

    // Validate environment variables
    if host == "" {
        return fmt.Errorf("HOST environment variable not set")
    }
    if port == "" {
        port = "5432" // default PostgreSQL port
    }
    if dbName == "" {
        return fmt.Errorf("DB_NAME environment variable not set")
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

    // Close existing connection if any
    if db != nil {
        db.Close()
    }

    // Open database connection (use pgx driver, store in GLOBAL db variable)
    db, err = sql.Open("pgx", dsn)
    if err != nil {
        return fmt.Errorf("failed to open database: %w", err)
    }

    // Test the connection
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := db.PingContext(ctx); err != nil {
        return fmt.Errorf("failed to ping database: %w", err)
    }
    log.Println("Database connection established")

    // Configure connection pool
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)

    // Create queries object
    dbQueries = queries.New(db)
    log.Println("Database queries initialized")

    // Update last refresh time
    credentialsLastRefresh = time.Now()

    return nil
}

// refreshCredentialsIfNeeded checks if credentials should be refreshed based on TTL
func refreshCredentialsIfNeeded() error {
    credentialsMu.Lock()
    needsRefresh := time.Since(credentialsLastRefresh) > credentialsTTL
    credentialsMu.Unlock()

    if needsRefresh {
        log.Println("Credentials TTL expired, refreshing...")
        return connectDB()
    }
    return nil
}

// isAuthError checks if the error is a database authentication error
func isAuthError(err error) bool {
    if err == nil {
        return false
    }
    errStr := err.Error()
    return strings.Contains(errStr, "password authentication failed") ||
           strings.Contains(errStr, "SQLSTATE 28P01") ||
           strings.Contains(errStr, "SQLSTATE 28000")
}

func init() {
    log.Println("Initializing Lambda function...")

    // Initial database connection
    if err := connectDB(); err != nil {
        log.Fatalf("Failed to connect to database: %v", err)
    }

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
    // Check if credentials need refreshing based on TTL
    if err := refreshCredentialsIfNeeded(); err != nil {
        log.Printf("Warning: failed to refresh credentials: %v", err)
    }

    limit := int32(50) // default
    offset := int32(0) // default

    // Try to fetch tenants, retry with fresh credentials on auth error
    tenants, err := dbQueries.ListTenants(ctx, queries.ListTenantsParams{
        Limit:  limit,
        Offset: offset,
    })
    if err != nil {
        // If auth error, refresh credentials and retry once
        if isAuthError(err) {
            log.Println("Authentication error detected, refreshing credentials...")
            if refreshErr := connectDB(); refreshErr != nil {
                return fmt.Errorf("failed to refresh credentials: %w", refreshErr)
            }
            // Retry the query
            tenants, err = dbQueries.ListTenants(ctx, queries.ListTenantsParams{
                Limit:  limit,
                Offset: offset,
            })
            if err != nil {
                return fmt.Errorf("failed to fetch tenants after credential refresh: %w", err)
            }
        } else {
            return fmt.Errorf("failed to fetch tenants: %w", err)
        }
    }
    log.Printf("Got tenants: %v", tenants)

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

    // Parse csv_field_config from payload for deliver_to_buyer filtering
    type CsvFieldConfigEntry struct {
        Key            string `json:"key"`
        Label          string `json:"label"`
        DeliverToBuyer bool   `json:"deliver_to_buyer"`
        Required       bool   `json:"required"`
        Order          int32  `json:"order"`
    }
    var csvFieldConfig []CsvFieldConfigEntry
    if cfgData, ok := payload["csv_field_config"].([]interface{}); ok && len(cfgData) > 0 {
        for _, cfgInterface := range cfgData {
            if cfgMap, ok := cfgInterface.(map[string]interface{}); ok {
                entry := CsvFieldConfigEntry{}
                if key, ok := cfgMap["key"].(string); ok {
                    entry.Key = key
                }
                if label, ok := cfgMap["label"].(string); ok {
                    entry.Label = label
                }
                if dtb, ok := cfgMap["deliver_to_buyer"].(bool); ok {
                    entry.DeliverToBuyer = dtb
                }
                if req, ok := cfgMap["required"].(bool); ok {
                    entry.Required = req
                }
                if order, ok := cfgMap["order"].(float64); ok {
                    entry.Order = int32(order)
                }
                csvFieldConfig = append(csvFieldConfig, entry)
            }
        }
        log.Printf("Parsed %d csv_field_config entries", len(csvFieldConfig))
    }

    // Build a set of field keys that should be delivered to the buyer
    deliverToBuyerKeys := make(map[string]bool)
    hasCsvFieldConfig := len(csvFieldConfig) > 0
    if hasCsvFieldConfig {
        for _, cfg := range csvFieldConfig {
            if cfg.DeliverToBuyer {
                deliverToBuyerKeys[cfg.Key] = true
            }
        }
        log.Printf("deliver_to_buyer keys: %v", deliverToBuyerKeys)
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

    // Helper function to extract custom answers from a lead
    extractCustomAnswers := func(lead map[string]interface{}) map[string]interface{} {
        if ca, ok := lead["CustomAnswers"]; ok && ca != nil {
            if caMap, ok := ca.(map[string]interface{}); ok {
                if rawMsg, hasRaw := caMap["RawMessage"]; hasRaw {
                    if valid, hasValid := caMap["Valid"]; hasValid {
                        if validBool, ok := valid.(bool); ok && validBool {
                            if rawMap, ok := rawMsg.(map[string]interface{}); ok {
                                return rawMap
                            }
                        }
                    }
                } else {
                    return caMap
                }
            }
        }
        return nil
    }

    // Define column structure for dynamic CSV generation
    type ColumnDef struct {
        Key       string
        Header    string
        Extractor func(lead map[string]interface{}) string
    }

    // Build column definitions for base fields
    baseColumns := []ColumnDef{
        {Key: "email", Header: "Email", Extractor: func(l map[string]interface{}) string { return extractString(l["EmailHash"]) }},
        {Key: "first_name", Header: "First Name", Extractor: func(l map[string]interface{}) string { return extractString(l["FirstName"]) }},
        {Key: "last_name", Header: "Last Name", Extractor: func(l map[string]interface{}) string { return extractString(l["LastName"]) }},
        {Key: "phone", Header: "Phone Number", Extractor: func(l map[string]interface{}) string { return extractString(l["PhoneHash"]) }},
        {Key: "company_name", Header: "Company Name", Extractor: func(l map[string]interface{}) string { return extractString(l["CompanyName"]) }},
        {Key: "employee_size", Header: "Employee Size", Extractor: func(l map[string]interface{}) string { return extractString(l["EmployeeSize"]) }},
        {Key: "publisher_name", Header: "Publisher Name", Extractor: func(l map[string]interface{}) string { return extractString(l["PublisherName"]) }},
        {Key: "linkedin_company", Header: "LinkedIn Company", Extractor: func(l map[string]interface{}) string { return extractString(l["LinkedinCompany"]) }},
        {Key: "linkedin_contact", Header: "LinkedIn Contact", Extractor: func(l map[string]interface{}) string { return extractString(l["LinkedinContact"]) }},
        {Key: "asset_name", Header: "Downloaded Asset Name", Extractor: func(l map[string]interface{}) string { return extractString(l["DownloadedAssetName"]) }},
        {Key: "state", Header: "State", Extractor: func(l map[string]interface{}) string { return extractString(l["State"]) }},
        {Key: "address", Header: "Address", Extractor: func(l map[string]interface{}) string { return extractString(l["Address"]) }},
        {Key: "industry", Header: "Industry", Extractor: func(l map[string]interface{}) string { return extractString(l["Industry"]) }},
        {Key: "ip_address", Header: "IP Address", Extractor: func(l map[string]interface{}) string { return extractIPAddress(l["IpAddress"]) }},
        {Key: "timestamp", Header: "Date/Time Stamp", Extractor: func(l map[string]interface{}) string { return extractTimestamp(l["CapturedAt"]) }},
        {Key: "naics_code", Header: "NAICS Code", Extractor: func(l map[string]interface{}) string { return extractString(l["NaicsCode"]) }},
    }

    // First pass: extract all values and track which columns have data
    type LeadValues struct {
        BaseValues     []string
        QuestionValues []string
    }
    allLeadValues := make([]LeadValues, 0, len(leadsData))
    columnHasData := make([]bool, len(baseColumns))
    questionHasData := make([]bool, len(questions))

    for _, leadInterface := range leadsData {
        lead := leadInterface.(map[string]interface{})

        // Extract base column values
        baseValues := make([]string, len(baseColumns))
        for i, col := range baseColumns {
            val := col.Extractor(lead)
            baseValues[i] = val
            if val != "" {
                columnHasData[i] = true
            }
        }

        // Extract question values
        customAnswers := extractCustomAnswers(lead)
        questionValues := make([]string, len(questions))
        for i, q := range questions {
            answer := ""
            if customAnswers != nil {
                if val, exists := customAnswers[q.ID]; exists && val != nil {
                    answer = extractString(val)
                }
            }
            questionValues[i] = answer
            if answer != "" {
                questionHasData[i] = true
            }
        }

        allLeadValues = append(allLeadValues, LeadValues{
            BaseValues:     baseValues,
            QuestionValues: questionValues,
        })
    }

    // Build header with only columns that should be delivered
    var header []string
    var includedBaseColumns []int
    var includedQuestionColumns []int

    for i, hasData := range columnHasData {
        if !hasData {
            continue
        }
        // If csv_field_config is present, only include columns where deliver_to_buyer is true
        if hasCsvFieldConfig {
            if !deliverToBuyerKeys[baseColumns[i].Key] {
                continue
            }
        }
        header = append(header, baseColumns[i].Header)
        includedBaseColumns = append(includedBaseColumns, i)
    }
    for i, hasData := range questionHasData {
        if hasData {
            header = append(header, questions[i].QuestionText)
            includedQuestionColumns = append(includedQuestionColumns, i)
        }
    }

    log.Printf("CSV will include %d base columns (of %d) and %d question columns (of %d)",
        len(includedBaseColumns), len(baseColumns), len(includedQuestionColumns), len(questions))
    log.Printf("Header columns: %v", header)

    // Generate CSV
    csvBuffer := new(bytes.Buffer)
    writer := csv.NewWriter(csvBuffer)
    writer.Write(header)

    // Write lead data with only included columns
    for _, leadValues := range allLeadValues {
        var row []string
        for _, idx := range includedBaseColumns {
            row = append(row, leadValues.BaseValues[idx])
        }
        for _, idx := range includedQuestionColumns {
            row = append(row, leadValues.QuestionValues[idx])
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
    const SenderAddress = "notifications@mail.lead-ship.com"
    

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
