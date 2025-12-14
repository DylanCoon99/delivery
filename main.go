package main

import (
    "context"
    "database/sql"
    "fmt"
    "log"
    "os"
    "net/url"
    "time"
    //"bytes"
    //"net/http"
    //"encoding/json"
    "github.com/google/uuid"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/ses/types"
    "github.com/aws/aws-sdk-go-v2/service/ses"
    _ "github.com/jackc/pgx/v5/stdlib"
    //"github.com/DylanCoon99/delivery/cmd/types"
    "github.com/DylanCoon99/delivery/internal/utils"
    "github.com/DylanCoon99/delivery/internal/database/queries"

)

var (
    db        *sql.DB          // Add this global variable
    dbQueries *queries.Queries
    sesClient *ses.Client
)

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
    method, err := q.GetDeliveryMethod(ctx, queries.GetDeliveryMethodParams{
        ID:       job.DeliveryMethodID,
        TenantID: job.TenantID,
    })
    
    if err != nil {
        return fmt.Errorf("failed to fetch delivery method: %w", err)
    }

    // Fetch buyer to get contact email
    buyer, err := q.GetBuyerByID(ctx, queries.GetBuyerByIDParams{
        ID:       job.BuyerID,
        TenantID: job.TenantID,
    })

    if err != nil {
        return fmt.Errorf("failed to fetch buyer: %w", err)
    }

    // Execute delivery
    var deliveryErr error

    switch method.MethodType.String {
    case "email":
        to := buyer.ContactEmail.String
        deliveryErr = deliverEmail(ctx, job, &method, to)
        log.Println("Finished Sending Email")
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
    status := sql.NullString{String: "completed", Valid: true}
    lastErr := sql.NullString{Valid: false}

    log.Printf("DeliveryErr: %v", deliveryErr)

    if deliveryErr != nil {
        status.String = "failed"
        lastErr = sql.NullString{String: deliveryErr.Error(), Valid: true}
    }

    if _, err := q.UpdateDeliveryJobStatus(ctx, queries.UpdateDeliveryJobStatusParams{
        ID:        job.ID,
        Status:    status.String,
        TenantID:  job.TenantID,
    }); err != nil {
        return fmt.Errorf("failed to update job status: %w", err)
    }

    // Add history
    if _, err := q.CreateDeliveryHistory(ctx, queries.CreateDeliveryHistoryParams{
        ID:                uuid.New(),
        JobID:             utils.NullUUID(job.ID),
        BuyerID:           utils.NullUUID(job.BuyerID),
        DeliveryMethodID:  utils.NullUUID(job.DeliveryMethodID),
        Status:            utils.SqlNullString(status.String),
        ErrorMessage:      utils.SqlNullString(lastErr.String),
    }); err != nil {
        return fmt.Errorf("failed to insert history: %w", err)
    }

    return deliveryErr
}

// SES email sender
func deliverEmail(ctx context.Context, job *queries.DeliveryJob, method *queries.DeliveryMethod, to string) error {
    const SenderAddress = "notifications@lead-ship.app"

    _, err := sesClient.SendEmail(ctx, &ses.SendEmailInput{
        Destination: &types.Destination{
            ToAddresses: []string{to},
        },
        Message: &types.Message{
            Subject: &types.Content{Data: aws.String("New Lead")},
            Body: &types.Body{
                Text: &types.Content{
                    Data: aws.String("Here are your leads. (TODO)"),
                },
            },
        },
        Source: aws.String(SenderAddress),
    })
    return err
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
