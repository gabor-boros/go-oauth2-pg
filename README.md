# go-oauth2-pg

[![GoDoc](https://godoc.org/github.com/gabor-boros/go-oauth2-pg?status.svg)](https://godoc.org/github.com/gabor-boros/go-oauth2-pg)
[![Go Report Card](https://goreportcard.com/badge/github.com/gabor-boros/go-oauth2-pg)](https://goreportcard.com/report/github.com/gabor-boros/go-oauth2-pg)
[![Maintainability](https://api.codeclimate.com/v1/badges/28517f7d90ff37bb27a0/maintainability)](https://codeclimate.com/github/gabor-boros/go-oauth2-pg/maintainability)
[![Test Coverage](https://api.codeclimate.com/v1/badges/28517f7d90ff37bb27a0/test_coverage)](https://codeclimate.com/github/gabor-boros/go-oauth2-pg/test_coverage)

This package is a [Postgres] storage implementation for [go-oauth2] using
[pgx].

The package is following semantic versioning and is not tied to the versioning
of [go-oauth2].

[Postgres]: https://www.postgresql.org/
[go-oauth2]: https://github.com/go-oauth2/oauth2
[pgx]: https://github.com/jackc/pgx

## Installation

```bash
go get github.com/gabor-boros/go-oauth2-pg
```

## Example usage

```go
package main

import (
	"context"
	"os"

	arangoDriver "github.com/pg/go-driver"
	arangoHTTP "github.com/pg/go-driver/http"

	"github.com/go-oauth2/oauth2/v4/manage"

	pgstore "github.com/gabor-boros/go-oauth2-pg"
)

func main() {
	conn, _ := arangoHTTP.NewConnection(arangoHTTP.ConnectionConfig{
		Endpoints: []string{os.Getenv("ARANGO_URL")},
	})

	client, _ := arangoDriver.NewClient(arangoDriver.ClientConfig{
		Connection:     conn,
		Authentication: arangoDriver.BasicAuthentication(os.Getenv("ARANGO_USER"), os.Getenv("ARANGO_PASSWORD")),
	})

	db, _ := client.Database(context.Background(), os.Getenv("ARANGO_DB"))
	
	clientStore, _ := pgstore.NewClientStore(
		pgstore.WithClientStoreDatabase(db),
		pgstore.WithClientStoreTable("oauth2_clients"),
	)

	tokenStore, _ := pgstore.NewTokenStore(
		pgstore.WithTokenStoreDatabase(db),
		pgstore.WithTokenStoreTable("oauth2_tokens"),
	)

	manager := manage.NewDefaultManager()
	manager.MapTokenStorage(tokenStore)
	manager.MapClientStorage(clientStore)
	
	// ...
}
```

## Contributing

Contributions are welcome! Please open an issue or a pull request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file
for details.
