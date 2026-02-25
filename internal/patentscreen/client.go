package patentscreen

import "github.com/joelkehle/techtransfer-agency/internal/busclient"

type Attachment = busclient.Attachment
type InboxEvent = busclient.InboxEvent
type Client = busclient.Client

var NewClient = busclient.NewClient
