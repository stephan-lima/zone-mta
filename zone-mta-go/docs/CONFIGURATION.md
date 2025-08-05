# Configuration Guide for ZoneMTA-Go

ZoneMTA-Go supports flexible configuration management with multiple YAML files, environment-specific overrides, and hierarchical configuration loading.

## Configuration Methods

### 1. Single File Configuration (Default)

The traditional approach using a single configuration file:

```bash
./zone-mta -config config/default.yaml
```

### 2. Environment-Specific Configuration

Load base configuration with environment-specific overrides:

```bash
# Loads default.yaml + production.yaml
./zone-mta -env production

# Loads default.yaml + development.yaml  
./zone-mta -env development

# Loads default.yaml + staging.yaml
./zone-mta -env staging
```

### 3. Directory-Based Loading

Load all YAML files from a directory with automatic prioritization:

```bash
# Loads all .yaml/.yml files from config/ directory
./zone-mta -config-dir config
```

### 4. Multiple Specific Files

Explicitly specify multiple files to merge:

```bash
# Load and merge specific files in order
./zone-mta -config-files "config/default.yaml,config/production.yaml,config/secrets.yaml"
```

## File Loading Priority

When using directory-based loading or automatic file discovery, files are loaded in this order:

1. **Base Configuration**
   - `default.yaml` / `default.yml`
   - `base.yaml` / `base.yml`

2. **Environment-Specific**
   - `production.yaml` / `production.yml`
   - `staging.yaml` / `staging.yml`
   - `development.yaml` / `development.yml`
   - `dev.yaml` / `dev.yml`
   - `test.yaml` / `test.yml`

3. **Local Overrides**
   - `local.yaml` / `local.yml`
   - `override.yaml` / `override.yml`

4. **Secrets**
   - `secrets.yaml` / `secrets.yml`

5. **Other files** (alphabetically)

Later files override values from earlier files.

## Configuration Structure

### Base Configuration (`config/default.yaml`)

Contains all default settings and serves as the foundation:

```yaml
app:
  name: "ZoneMTA-Go"
  ident: "zone-mta-go"

databases:
  mongodb:
    uri: "mongodb://127.0.0.1:27017/zone-mta-go"
    database: "zone-mta-go"
    timeout: 10s
  redis:
    host: "127.0.0.1"
    port: 6379
    db: 3

# ... full configuration
```

### Environment Configuration (`config/production.yaml`)

Override specific values for production:

```yaml
app:
  name: "ZoneMTA-Go Production"

databases:
  mongodb:
    uri: "mongodb://mongo-cluster:27017/zone-mta-production"
  redis:
    host: "redis-cluster"

logging:
  level: "info"
  format: "json"

smtp:
  interfaces:
    feeder:
      host: "0.0.0.0"  # Listen on all interfaces
      maxConnections: 500
```

### Local Overrides (`config/local.yaml`)

Personal development settings (not committed to version control):

```yaml
app:
  name: "ZoneMTA-Go Local"

databases:
  mongodb:
    database: "zone-mta-local"

logging:
  level: "debug"

plugins:
  message-logger:
    enabled: true
    logLevel: "debug"
```

### Secrets (`config/secrets.yaml`)

Sensitive configuration (never committed):

```yaml
databases:
  mongodb:
    uri: "mongodb://username:password@mongo-server:27017/zone-mta?authSource=admin"

plugins:
  dkim-sign:
    keys:
      example.com:
        privateKey: |
          -----BEGIN PRIVATE KEY-----
          ...
          -----END PRIVATE KEY-----
```

## Configuration Examples

### Development Setup

```bash
# Start with development configuration
./zone-mta -env development

# Or load directory with development overrides
./zone-mta -config-dir config
```

File loading order for development:
1. `config/default.yaml` (base settings)
2. `config/development.yaml` (dev-specific overrides)
3. `config/local.yaml` (personal overrides, if exists)

### Production Setup

```bash
# Production with environment-specific config
./zone-mta -env production

# Production with explicit files
./zone-mta -config-files "config/default.yaml,config/production.yaml,config/secrets.yaml"
```

### Docker Deployment

```dockerfile
# Use environment-specific configuration
CMD ["./zone-mta", "-env", "production"]

# Or mount specific config files
VOLUME ["/app/config"]
CMD ["./zone-mta", "-config-dir", "/app/config"]
```

## Configuration Merging Rules

### Primitive Values
Simple values (strings, numbers, booleans) are completely replaced:

```yaml
# Base
logging:
  level: "info"

# Override  
logging:
  level: "debug"  # Replaces "info"
```

### Objects/Maps
Objects are merged recursively:

```yaml
# Base
databases:
  mongodb:
    uri: "mongodb://localhost:27017/db"
    timeout: 10s
  redis:
    host: "localhost"

# Override
databases:
  mongodb:
    uri: "mongodb://prod-server:27017/db"  # Replaces URI
    # timeout remains 10s
  # redis config remains unchanged
```

### Arrays/Lists
Arrays are completely replaced (not merged):

```yaml
# Base
plugins:
  rate-limiter:
    whitelistIp:
      - "127.0.0.1"
      - "::1"

# Override
plugins:
  rate-limiter:
    whitelistIp:
      - "10.0.0.0/8"  # Completely replaces the array
```

## Validation and Debugging

### Show Configuration

Display the final merged configuration:

```bash
# Show final config
./zone-mta -show-config -env production

# Show which files would be loaded
./zone-mta -show-files -config-dir config
```

### Validate Configuration

```bash
# Test configuration without starting services
./zone-mta -show-config -config-files "config/default.yaml,config/production.yaml"
```

### Common Issues

1. **File Not Found**: Ensure all specified files exist
2. **YAML Syntax**: Validate YAML syntax in each file
3. **Override Order**: Remember that later files override earlier ones
4. **Missing Base**: Environment configs should override, not replace base config

## Best Practices

### 1. Environment Strategy

```
config/
├── default.yaml      # Base configuration (committed)
├── development.yaml  # Dev overrides (committed)
├── production.yaml   # Prod overrides (committed)
├── staging.yaml      # Staging overrides (committed)
├── local.yaml        # Local dev overrides (gitignored)
└── secrets.yaml      # Secrets (gitignored)
```

### 2. Docker Strategy

```yaml
# docker-compose.yml
services:
  zone-mta:
    image: zone-mta-go
    command: ["./zone-mta", "-env", "production"]
    volumes:
      - ./config:/app/config:ro
    environment:
      - ENV=production
```

### 3. Kubernetes Strategy

```yaml
# ConfigMap for base config
apiVersion: v1
kind: ConfigMap
metadata:
  name: zone-mta-config
data:
  default.yaml: |
    # base configuration
  production.yaml: |
    # production overrides

---
# Secret for sensitive data
apiVersion: v1
kind: Secret
metadata:
  name: zone-mta-secrets
data:
  secrets.yaml: <base64-encoded-secrets>
```

### 4. Security

- Never commit `secrets.yaml` or files containing credentials
- Use environment variables for sensitive data when possible
- Restrict file permissions on configuration files
- Use separate configuration for different environments

## Environment Variables

You can also use environment variables in configuration files:

```yaml
databases:
  mongodb:
    uri: "${MONGODB_URI:-mongodb://localhost:27017/zone-mta}"
  redis:
    host: "${REDIS_HOST:-localhost}"
    port: ${REDIS_PORT:-6379}
```

## Makefile Helpers

The project includes Makefile targets for configuration management:

```bash
# Show configuration files that would be loaded
make show-config-files

# Show development configuration
make show-config-dev

# Show production configuration  
make show-config-prod

# Test multi-file loading
make test-config-multi
```

## Migration from Single File

To migrate from single-file to multi-file configuration:

1. **Keep existing file** as `config/default.yaml`
2. **Create environment files** with only overrides
3. **Extract secrets** to separate file
4. **Update startup commands** to use new flags
5. **Test thoroughly** with `-show-config`

This approach provides maximum flexibility while maintaining backward compatibility.