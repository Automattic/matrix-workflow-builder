package bot

import (
	"fmt"
	"neurobot/infrastructure/matrix"
	model "neurobot/model/bot"
	"strings"
)

type Registry interface {
	Append(bot model.Bot, client matrix.Client) error
	GetClient(identifier string) matrix.Client
}

type registry struct {
	homeserverDomain string
	clients          map[string]matrix.Client
}

func NewRegistry(homeserverURL string) *registry {
	return &registry{
		// Remove port to get just the domain
		homeserverDomain: strings.Split(homeserverURL, ":")[0],
	}
}

func (r *registry) Append(bot model.Bot, client matrix.Client) (err error) {
	if _, ok := r.clients[bot.Identifier]; ok {
		return fmt.Errorf("bot %s is already known", bot.Identifier)
	}

	if err = client.Login(bot.Username, bot.Password); err != nil {
		return
	}

	r.clients[bot.Identifier] = client

	return
}

func (r *registry) GetClient(identifier string) matrix.Client {
	if client, ok := r.clients[identifier]; ok {
		return client
	}

	return nil
}
