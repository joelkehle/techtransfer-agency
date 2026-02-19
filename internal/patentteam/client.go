package patentteam

import "github.com/joelkehle/agent-bus-v2/internal/busclient"

// Type aliases so existing patentteam code compiles unchanged.
type Attachment = busclient.Attachment
type InboxEvent = busclient.InboxEvent
type Client = busclient.Client

var NewClient = busclient.NewClient
