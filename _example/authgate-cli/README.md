# AuthGate CLI - OAuth 2.0 Device Flow Client

AuthGate CLI is a command-line tool that demonstrates how to authenticate with AuthGate server using the OAuth 2.0 Device Authorization Flow.

## What does it do?

This tool allows you to:

- Authenticate CLI applications without requiring a web browser redirect
- Securely store and reuse access tokens
- Automatically refresh expired tokens
- Verify token validity

Perfect for headless environments, SSH sessions, or any scenario where traditional OAuth flows are impractical.

## Quick Start

### 1. Install

```bash
# Clone the repository
git clone <repository-url>
cd authgate/_example/authgate-cli

# Build the tool
go build -o authgate-cli
```

### 2. Get Your Client ID

Start the AuthGate server and look for the client ID in the startup logs:

```txt
Client ID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

### 3. Run the Tool

```bash
# Using command-line flag (recommended)
./authgate-cli -client-id=<your-client-id>

# Or using environment variable
export CLIENT_ID=<your-client-id>
./authgate-cli
```

## Configuration

You can configure the tool in three ways (in order of priority):

### Option 1: Command-Line Flags (Highest Priority)

```bash
./authgate-cli -client-id=abc-123 \
   -server-url=http://localhost:8080 \
   -token-file=./my-tokens.json
```

### Option 2: Environment Variables

```bash
export CLIENT_ID=abc-123
export SERVER_URL=http://localhost:8080
export TOKEN_FILE=.authgate-tokens.json
./authgate-cli
```

### Option 3: .env File

Create a `.env` file:

```env
CLIENT_ID=abc-123
SERVER_URL=http://localhost:8080
TOKEN_FILE=.authgate-tokens.json
```

Then run:

```bash
./authgate-cli
```

## How to Use

### First Time Login

1. Run the tool with your client ID
2. You'll see a URL and a user code:

   ```txt
   Please open this link to authorize:
   http://localhost:8080/device?user_code=ABC12345

   Or manually visit: http://localhost:8080/device
   And enter code: ABC12345
   ```

3. Open the URL in your browser
4. Login to AuthGate (default: admin / check server logs for password)
5. Enter the user code when prompted
6. The CLI will automatically detect authorization and save your tokens

### Subsequent Uses

After the first login, your tokens are saved locally. The tool will:

- Reuse valid access tokens
- Automatically refresh expired access tokens
- Request new authorization only if refresh fails

### Token Storage

Tokens are saved in `.authgate-tokens.json` by default (or the path you specify). This file now supports **multiple Client IDs**, allowing you to manage tokens for different OAuth clients in a single file.

File structure:

```json
{
  "tokens": {
    "client-id-1": {
      "access_token": "...",
      "refresh_token": "...",
      "token_type": "Bearer",
      "expires_at": "2026-01-20T12:00:00Z",
      "client_id": "client-id-1"
    },
    "client-id-2": {
      "access_token": "...",
      "refresh_token": "...",
      "token_type": "Bearer",
      "expires_at": "2026-01-20T13:00:00Z",
      "client_id": "client-id-2"
    }
  }
}
```

Each client ID has its own token entry containing:

- Access token
- Refresh token
- Token type
- Token expiration time
- Client ID

**Security Note**: This file is created with `0600` permissions (owner read/write only).

## Usage Examples

### Basic Usage

```bash
# First run - will prompt for authorization
./authgate-cli -client-id=abc-123

# Second run - will reuse saved tokens
./authgate-cli -client-id=abc-123
```

### Custom Server URL

```bash
# Connect to a different server
./authgate-cli -client-id=abc-123 -server-url=https://auth.example.com
```

### Multiple Client IDs

#### Same Token File (Recommended)

The default behavior now supports multiple clients in one file:

```bash
# Both clients save to the same .authgate-tokens.json file
./authgate-cli -client-id=abc-123
./authgate-cli -client-id=xyz-789

# Tokens for both clients are stored together and managed independently
```

#### Separate Token Files

You can still use different token files if preferred:

```bash
# Use different token files for different environments
./authgate-cli -client-id=abc-123 -token-file=./work-tokens.json
./authgate-cli -client-id=xyz-789 -token-file=./personal-tokens.json
```

### Help

```bash
# View all available options
./authgate-cli -h
```

## What Happens Behind the Scenes

1. **Device Code Request**: CLI requests a device code from the server
2. **User Authorization**: You authorize the device via web browser
3. **Token Exchange**: CLI polls the server and receives tokens once authorized
4. **Token Storage**: Tokens are saved locally for future use
5. **Automatic Refresh**: Expired tokens are refreshed automatically
6. **Token Verification**: Each run verifies the token is still valid

## Advanced Features

### Intelligent Polling with Exponential Backoff

The CLI implements RFC 8628-compliant polling with smart error handling:

#### Polling Behavior

- **Initial Interval**: Defaults to 5 seconds (configurable by server)
- **UI Updates**: Progress dots appear every 2 seconds (reduces visual clutter)
- **Automatic Formatting**: Adds a newline every 50 dots for readability

#### Error Handling

The CLI handles all OAuth 2.0 Device Authorization Grant error codes:

| Error Type              | Description                               | CLI Behavior                                                |
| ----------------------- | ----------------------------------------- | ----------------------------------------------------------- |
| `authorization_pending` | User hasn't authorized yet                | Continues polling, shows progress dots                      |
| `slow_down`             | Server requests slower polling            | **Triggers exponential backoff** (1.5x multiplier, max 60s) |
| `expired_token`         | Device code expired (default: 30 minutes) | Stops and prompts to restart authentication                 |
| `access_denied`         | User explicitly denied authorization      | Stops and displays denial message                           |
| Other errors            | Unexpected server errors                  | Stops and displays detailed error information               |

#### Exponential Backoff Example

When the server returns `slow_down`, the CLI automatically adjusts its polling interval:

```txt
Initial:  5 seconds
1st slow_down:  5 × 1.5 = 7.5 seconds
2nd slow_down:  7.5 × 1.5 = 11.25 seconds
3rd slow_down:  11.25 × 1.5 = 16.875 seconds
...
Maximum:  60 seconds (capped)
```

This prevents overwhelming the server while maintaining responsiveness.

#### Context Support

The polling mechanism respects context cancellation:

- Supports timeout configuration
- Graceful shutdown on interrupt (Ctrl+C)
- Proper cleanup of resources

### Security Features

The CLI includes several security enhancements:

#### HTTP Client Security

- **Timeout Protection**: All requests timeout after 30 seconds (prevents hanging)
- **TLS 1.2+**: Minimum TLS version enforced
- **HTTPS Warning**: Automatically warns when using HTTP (insecure)
- **Connection Pooling**: Efficient resource usage with idle connection management

#### Input Validation

- **SERVER_URL Validation**: Checks URL format and scheme (http/https only)
- **CLIENT_ID Validation**: Warns if CLIENT_ID is not a valid UUID
- **Error Messages**: Clear, actionable error messages without exposing sensitive data

#### Token File Security

- **File Permissions**: Created with `0600` (owner read/write only)
- **Atomic Writes**: Uses temporary file + rename pattern to prevent corruption
- **Concurrent Access**: File locking prevents race conditions when multiple processes access the same token file

#### Secure Error Handling

- **No Ignored Errors**: All error conditions are checked and handled
- **Wrapped Errors**: Error chains preserve context for debugging
- **Safe Logging**: Token values are truncated or redacted in logs

## Troubleshooting

### "CLIENT_ID not set" Error

Make sure you've provided the client ID via one of these methods:

- Command-line flag: `-client-id=<your-id>`
- Environment variable: `CLIENT_ID=<your-id>`
- `.env` file: `CLIENT_ID=<your-id>`

You can find your client ID in the AuthGate server startup logs.

### "Connection refused" Error

Make sure the AuthGate server is running:

```bash
# In another terminal
cd authgate
./bin/authgate server
```

Check that the server URL matches (default: `http://localhost:8080`).

### "Token verification failed" Error

Your token may have been revoked or expired. Delete the token file and re-authenticate:

```bash
rm .authgate-tokens.json
./authgate-cli -client-id=<your-id>
```

### "Refresh failed" Error

If token refresh fails, the tool will automatically start a new device flow. Follow the authorization steps again.

### "slow_down" Error During Polling

If you see slower progress dots during authorization, the CLI has detected a `slow_down` signal from the server and automatically adjusted its polling interval. This is normal behavior to prevent server overload.

### "context deadline exceeded" Error

This indicates the operation timed out (default: 30 seconds for requests). Check your network connection and server availability.

## Security Best Practices

1. **Protect Your Token File**: Never commit `.authgate-tokens.json` to version control. Add it to `.gitignore`
2. **Use HTTPS in Production**: The CLI will warn you if using HTTP. Always use HTTPS server URLs in production
3. **Secure Client ID**: While not a secret, treat your client ID as sensitive configuration
4. **Regular Cleanup**: Delete token files when no longer needed
5. **File Permissions**: The CLI automatically sets token file permissions to `0600` (owner-only access)
6. **Review Warnings**: Pay attention to security warnings about HTTP usage and invalid UUIDs

## Learn More

For more information about the OAuth 2.0 Device Authorization Grant flow:

- [RFC 8628 - OAuth 2.0 Device Authorization Grant](https://datatracker.ietf.org/doc/html/rfc8628)
- [AuthGate Documentation](../../README.md)

## Support

If you encounter any issues or have questions, please open an issue on the project repository.
