module amzckeepa

go 1.25.0

require (
	github.com/andybalholm/brotli v1.0.6
	github.com/go-sql-driver/mysql v1.10.0
	recloud.dev/cdncache v0.0.0
)

replace recloud.dev/cdncache => ../../cdncache

require filippo.io/edwards25519 v1.2.0 // indirect
