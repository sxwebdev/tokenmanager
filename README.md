# TokenManager Package

[![Go Reference](https://pkg.go.dev/badge/github.com/sxwebdev/tokenmanager.svg)](https://pkg.go.dev/github.com/sxwebdev/tokenmanager)
[![Go Version](https://img.shields.io/badge/go-1.25-blue)](https://go.dev/)
[![Go Report Card](https://goreportcard.com/badge/github.com/sxwebdev/tokenmanager)](https://goreportcard.com/report/github.com/sxwebdev/tokenmanager)
[![License](https://img.shields.io/github/license/sxwebdev/tokenmanager)](LICENSE)

The `tokenmanager` package provides a flexible and generic implementation for token management. It allows you to create, validate, and revoke signed tokens using an abstract storage interface (`ITokenStore`). This package supports additional data (using Go generics) to be stored along with token metadata, such as user ID, issue time, expiry, and token type.

## Features

- **Abstract Storage Interface (`ITokenStore`)**:  
  Supports any storage implementation (e.g., Redis, Memcached). An in‑memory implementation (`MemoryTokenStore`) is provided for testing and simple use cases.

- **Signed Tokens**:  
  Tokens are signed using HMAC-SHA256 to ensure integrity and to verify that they were issued by your backend.

- **Generic Additional Data**:  
  Token data can include additional custom information using Go generics.

- **Token Types**:  
  Supports different token types (e.g., access and refresh tokens).

## Installation

```bash
go get github.com/sxwebdev/tokenmanager@latest
```

## Usage

Below is a simple example demonstrating how to create a `TokenManager` with in‑memory storage and how to create, validate, and revoke tokens.

```go
package main

import (
  "fmt"
  "time"

  "github.com/sxwebdev/tokenmanager"
)

func main() {
  // Initialize the in-memory token store.
  store := tokenmanager.NewMemoryTokenStore()

  ctx := context.Background()

  // Create a new TokenManager with a secret key.
  // Here, we use `any` for the additional data type, but you can replace it with a custom type.
  manager := tokenmanager.NewTokenManager[map[string]any](store, "your_very_secret_key", time.Minute * 15)

  userAdditionalData := map[string]any{
    "username": "John Doe",
    "isActive": true,
    "age": 38,
  }

  // Create an access token with additional data (can be nil or a custom type).
  token, err := manager.CreateToken(ctx, "user123", userAdditionalData, tokenmanager.AccessTokenType)
  if err != nil {
    panic(err)
  }
  fmt.Println("Created token:", token)

  // Validate the token.
  data, valid := manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
  if !valid {
    fmt.Println("Token is invalid or expired")
    return
  }
  fmt.Printf("Token is valid. UserID: %s, IssuedAt: %s, Expires: %s\n", data.UserID, data.IssuedAt, data.Expiry)

  // Revoke the token.
  manager.RevokeToken(token)

  // Try validating the revoked token.
  _, valid = manager.ValidateToken(ctx, token, tokenmanager.AccessTokenType)
  if !valid {
    fmt.Println("Token has been successfully revoked.")
  } else {
    fmt.Println("Token is still valid.")
  }
}
```

## Running Tests

The package includes tests for token creation, validation, expiration, and revocation. To run the tests, execute the following command in your terminal:

```bash
go test -v ./...
```

## License

This package is provided as-is without any warranty. Use it at your own risk.

```text
---

This README now reflects the module path `github.com/sxwebdev/tokenmanager`.
```
