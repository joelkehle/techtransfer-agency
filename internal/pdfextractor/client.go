package pdfextractor

import "github.com/joelkehle/techtransfer-agency/pkg/busclient"

type Client = busclient.Client
type InboxEvent = busclient.InboxEvent

var NewClient = busclient.NewClient
