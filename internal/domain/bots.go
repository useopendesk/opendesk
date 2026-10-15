package domain

import (
	"encoding/json"
	"hash/fnv"
	"strconv"

	"github.com/google/uuid"
)

// BotRef is the bot an AI reply was sent as (contracts/bots.md "Where a bot shows"):
// its name, a PNG of its look (or its uploaded picture) and the look itself, so clients
// that render looks draw it the same way.
type BotRef struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	AvatarURL string          `json:"avatar_url" doc:"PNG of the bot's look, or its uploaded picture"`
	Look      json.RawMessage `json:"look" doc:"BotLook (contracts/bots.md)"`
}

// BotAvatarURL is the picture of a bot for clients that cannot render its look: the
// uploaded picture when it has one and shows it, else the public PNG of the look.
// look is the stored look document; its hash keeps caches fresh when it changes.
func BotAvatarURL(id uuid.UUID, avatar Avatar, look []byte) string {
	var k struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(look, &k)
	if u := avatar.URL(); u != "" && k.Kind == "image" {
		return u
	}
	h := fnv.New32a()
	_, _ = h.Write(look)
	return apiURL + "/v1/public/bots/" + id.String() + "/avatar.png?v=" + strconv.FormatUint(uint64(h.Sum32()), 16)
}

// PublicURL is a path on this API's public base URL.
func PublicURL(path string) string { return apiURL + path }
