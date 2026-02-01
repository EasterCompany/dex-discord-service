module github.com/EasterCompany/dex-discord-service

go 1.25.6

require (
	github.com/EasterCompany/dex-go-utils v0.0.0
	github.com/bwmarrin/discordgo v0.29.1-0.20251229161010-9f6aa8159fc6
	github.com/redis/go-redis/v9 v9.17.3
	layeh.com/gopus v0.0.0-20210501142526-1ee02d434e32
)

replace github.com/EasterCompany/dex-go-utils => ../dex-go-utils

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
)
