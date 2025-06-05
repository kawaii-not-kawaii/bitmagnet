package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel/gen"
	"github.com/hekmon/transmissionrpc/v3"
)

type transmissionClient struct {
	CommonClient
}

func (c transmissionClient) sendTo(ctx context.Context, content *content) error {
	sendTo, ok := c.config.GetSendTo(gen.ClientIDTransmission)
	if !ok {
		return nil
	}

	endpoint, err := url.Parse(
		fmt.Sprintf("http://%v:%v/transmission/rpc", sendTo.Host, sendTo.Port))
	if err != nil {
		return err
	}

	tbt, err := transmissionrpc.New(endpoint, nil)
	if err != nil {
		return err
	}

	settings, err := tbt.SessionArgumentsGetAll(ctx)
	if err != nil {
		return err
	}

	for _, item := range *content {
		category := c.downloadCategory(item.Content.Type)

		dir := *settings.DownloadDir + "/" + category

		magnet := item.Torrent.MagnetURI()

		_, err = tbt.TorrentAdd(ctx, transmissionrpc.TorrentAddPayload{
			Filename:    &magnet,
			DownloadDir: &dir,
		})
		if err != nil {
			return err
		}
	}

	return nil
}
