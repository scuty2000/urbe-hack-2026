package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/ardanlabs/conf/v3/yaml"
)

const prefix = "API"

// WebAPIConfiguration describes the web API configuration. This structure is automatically parsed by
// loadConfiguration and values from flags, environment variable or configuration file will be loaded.
type WebAPIConfiguration struct {
	Config struct {
		Path string `conf:"default:/config/config.yml,short:c,help:path to the YAML configuration file"`
	}
	Web struct {
		APIHost         string        `conf:"env:WEB_APIHOST,default:0.0.0.0:3000,short:H,help:host:port the REST API server listens on"`
		DebugHost       string        `conf:"env:WEB_DEBUGHOST,default:0.0.0.0:4000,help:host:port the debug/pprof server listens on"`
		ReadTimeout     time.Duration `conf:"env:WEB_READTIMEOUT,default:10s,help:maximum duration to read an entire HTTP request"`
		WriteTimeout    time.Duration `conf:"env:WEB_WRITETIMEOUT,default:60s,help:maximum duration to write an HTTP response"`
		ShutdownTimeout time.Duration `conf:"env:WEB_SHUTDOWNTIMEOUT,default:10s,help:maximum duration to wait for active connections during graceful shutdown"`
		TrustedProxies  []string      `conf:"env:WEB_TRUSTEDPROXIES,help:list of trusted proxy IP addresses or CIDR ranges"`
	}
	CORS struct {
		AllowAllOrigins  bool     `conf:"env:CORS_ALLOWALLORIGINS,default:false,help:allow requests from any origin"`
		AllowedOrigins   []string `conf:"env:CORS_ALLOWEDORIGINS,default:http://localhost:3000;http://localhost:5173,help:list of allowed origins"`
		AllowedMethods   []string `conf:"env:CORS_ALLOWEDMETHODS,default:GET;POST;PUT;PATCH;DELETE;OPTIONS,help:list of allowed HTTP methods"`
		AllowedHeaders   []string `conf:"env:CORS_ALLOWEDHEADERS,default:Accept;Content-Type;Authorization,help:list of allowed HTTP headers"`
		AllowCredentials bool     `conf:"env:CORS_ALLOWCREDENTIALS,default:true,help:allow cookies and credentials in cross-origin requests"`
		MaxAge           int      `conf:"env:CORS_MAXAGE,default:86400,help:max age in seconds for preflight cache"`
	}
	Debug bool `conf:"short:d,help:enable verbose debug logging"`
	DB    struct {
		Filename string `conf:"default:db/lostfound.db,help:path to the SQLite database file"`
	}
	Logger struct {
		Level string `conf:"default:info,help:log level (debug, info, warn, error)"`
		File  string `conf:"default:stdout,help:path to the log file, stdout or stderr"`
	}

	// DeepSeek is the orchestrator LLM used by the conversational agent.
	DeepSeek struct {
		Key           string        `conf:"env:DEEPSEEK_KEY,help:DeepSeek API key"`
		URL           string        `conf:"env:DEEPSEEK_URL,default:https://api.deepseek.com,help:DeepSeek API base URL"`
		Model         string        `conf:"env:DEEPSEEK_MODEL,default:deepseek-v4.1-flash,help:DeepSeek model name"`
		Timeout       time.Duration `conf:"env:DEEPSEEK_TIMEOUT,default:30s,help:maximum duration of a DeepSeek request"`
		MaxToolRounds int           `conf:"env:DEEPSEEK_MAX_TOOL_ROUNDS,default:5,help:maximum number of tool-calling rounds"`
	}

	// D3 is the structured decision engine used for item classification.
	D3 struct {
		URL     string        `conf:"env:D3_URL,help:D3 base URL"`
		Key     string        `conf:"env:D3_KEY,help:D3 API key"`
		Model   string        `conf:"env:D3_MODEL,default:d3,help:D3 model name"`
		Timeout time.Duration `conf:"env:D3_TIMEOUT,default:10s,help:maximum duration of a D3 request"`
		// AllowImages sends item photos to D3. It defaults to false: identity
		// and document images must never leave the perimeter.
		AllowImages bool `conf:"env:D3_ALLOW_IMAGES,default:false,help:send item photos to D3"`
	}

	// Embeddings points at the vLLM embedding endpoint (4096-d Nemotron).
	Embeddings struct {
		URL       string        `conf:"default:http://www.lucascutigliani.it:8000/v1/embeddings,help:embeddings endpoint URL"`
		Model     string        `conf:"default:nvidia/Nemotron-3.5-Embed-8B-BF16,help:embedding model name"`
		Key       string        `conf:"help:embeddings API key"`
		Timeout   time.Duration `conf:"default:5s,help:maximum duration of an embedding request"`
		BatchSize int           `conf:"env:EMBEDDINGS_BATCH_SIZE,default:32,help:number of texts per embedding request"`
	}

	// Qdrant is the vector database used for semantic item search.
	Qdrant struct {
		URL        string        `conf:"default:http://localhost:6333,help:Qdrant base URL"`
		Collection string        `conf:"default:items,help:Qdrant collection name"`
		APIKey     string        `conf:"env:QDRANT_API_KEY,help:Qdrant API key"`
		Timeout    time.Duration `conf:"default:5s,help:maximum duration of a Qdrant request"`
	}

	// Auth holds session and password hashing secrets.
	Auth struct {
		SessionSecret string        `conf:"env:AUTH_SESSION_SECRET,help:secret used to sign session tokens"`
		PassSecret    string        `conf:"env:AUTH_PASS_SECRET,help:secret used for password hashing"`
		SessionTTL    time.Duration `conf:"env:AUTH_SESSION_TTL,default:12h,help:session lifetime"`
		PassTTL       time.Duration `conf:"env:AUTH_PASS_TTL,default:30m,help:password reset token lifetime"`
		CookieName    string        `conf:"env:AUTH_COOKIE_NAME,default:lf_session,help:session cookie name"`
	}

	// Storage configures uploads on disk.
	Storage struct {
		DataDir     string `conf:"env:STORAGE_DATA_DIR,default:./data,help:directory for uploaded files"`
		MaxUploadMB int    `conf:"env:STORAGE_MAX_UPLOAD_MB,default:8,help:maximum upload size in megabytes"`
	}

	// Custody policy.
	Civic struct {
		CustodyDays int `conf:"env:CIVIC_CUSTODY_DAYS,default:7,help:days an item is held at the civic office"`
	}
	Sensitive struct {
		CustodyHours int `conf:"env:SENSITIVE_CUSTODY_HOURS,default:24,help:hours a sensitive item is held"`
	}
	Legal struct {
		TermDays int `conf:"env:LEGAL_TERM_DAYS,default:365,help:legal retention term in days"`
	}

	// PA holds the public administration fees.
	PA struct {
		DailyRateCents int    `conf:"env:PA_DAILY_RATE_CENTS,default:500,help:daily custody rate in cents"`
		FixedFeeCents  int    `conf:"env:PA_FIXED_FEE_CENTS,default:1000,help:fixed practice fee in cents"`
		Currency       string `conf:"env:PA_CURRENCY,default:EUR,help:currency code"`
	}

	// Payment configures the payment provider (mock or stripe).
	Payment struct {
		Provider string `conf:"default:mock,help:payment provider (mock or stripe)"`
	}
	Stripe struct {
		SecretKey string `conf:"env:STRIPE_SECRET_KEY,help:Stripe secret key"`
	}

	// Demo enables demo-only endpoints.
	Demo struct {
		ResetSecret string `conf:"env:DEMO_RESET_SECRET,help:secret required to reset demo data"`
		Mode        bool   `conf:"default:true,help:enable demo mode"`
	}

	// Features toggles optional integrations.
	Features struct {
		Qdrant   bool `conf:"env:FEATURES_QDRANT,default:true,help:enable Qdrant vector search"`
		D3       bool `conf:"env:FEATURES_D3,default:true,help:enable the D3 decision engine"`
		DeepSeek bool `conf:"env:FEATURES_DEEPSEEK,default:true,help:enable the DeepSeek orchestrator"`
		Payment  bool `conf:"env:FEATURES_PAYMENT,default:true,help:enable payments"`
	}

	// App holds public-facing application settings.
	App struct {
		PublicURL string `conf:"env:APP_PUBLIC_URL,default:http://localhost:3000,help:public base URL of the application"`
	}
}

// loadConfiguration creates a WebAPIConfiguration starting from flags, environment variables and configuration file.
// It works by loading the configuration file first (if it exists), then applying environment variables, and finally
// command line flags. So, CLI parameters will override the environment, which will override the configuration file.
// Note that the configuration file path can be specified only via CLI or environment variable.
func loadConfiguration() (WebAPIConfiguration, error) {
	var cfg WebAPIConfiguration

	// First pass: parse env/flags to discover the demo file path.
	help, err := conf.ParseWithOptions(prefix, &cfg, conf.WithStrictFlags())
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Println(help) //nolint:forbidigo
			return cfg, conf.ErrHelpWanted
		}
		return cfg, fmt.Errorf("parsing demo: %w", err)
	}

	// If a YAML demo file exists, reparse supplying the YAML data (useful in k8s/compose).
	// YAML is applied before env vars and flags, so env/flags take precedence.
	fp, err := os.Open(cfg.Config.Path)
	if err != nil && !os.IsNotExist(err) {
		return cfg, fmt.Errorf("can't read the demo file, while it exists: %w", err)
	} else if err == nil {
		yamlFile, err := io.ReadAll(fp)
		_ = fp.Close()
		if err != nil {
			return cfg, fmt.Errorf("can't read demo file: %w", err)
		}
		help, err = conf.Parse(prefix, &cfg, yaml.WithData(yamlFile))
		if err != nil {
			if errors.Is(err, conf.ErrHelpWanted) {
				fmt.Println(help) //nolint:forbidigo
				return cfg, conf.ErrHelpWanted
			}
			return cfg, fmt.Errorf("parsing demo with yaml: %w", err)
		}
	}

	return cfg, nil
}
