package pdfextractor

import "github.com/joelkehle/pinakes/pkg/busclient"

type Client = busclient.Client
type InboxEvent = busclient.InboxEvent

var NewClient = busclient.NewClient
