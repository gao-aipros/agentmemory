package main

import (
	"fmt"
	"os"

	_ "github.com/go-chi/chi/v5"
	_ "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/stretchr/testify"
	_ "github.com/testcontainers/testcontainers-go"
	_ "github.com/tmc/langchaingo"
)

func main() {
	fmt.Println("AgentMemory v2")
	os.Exit(0)
}
