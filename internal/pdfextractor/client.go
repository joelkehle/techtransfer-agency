package pdfextractor

import "github.com/joelkehle/techtransfer-agency/internal/busclient"

type Client = busclient.Client
type InboxEvent = busclient.InboxEvent

var NewClient = busclient.NewClient
