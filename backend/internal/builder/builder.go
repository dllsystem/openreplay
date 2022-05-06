package builder

import (
	"net/url"
	"strings"
	"time"

	"openreplay/backend/pkg/intervals"
	. "openreplay/backend/pkg/messages"
)

func getURLExtention(URL string) string {
	u, err := url.Parse(URL)
	if err != nil {
		return ""
	}
	i := strings.LastIndex(u.Path, ".")
	return u.Path[i+1:]
}

func getResourceType(initiator string, URL string) string {
	switch initiator {
	case "xmlhttprequest", "fetch":
		return "fetch"
	case "img":
		return "img"
	default:
		switch getURLExtention(URL) {
		case "css":
			return "stylesheet"
		case "js":
			return "script"
		case "png", "gif", "jpg", "jpeg", "svg":
			return "img"
		case "mp4", "mkv", "ogg", "webm", "avi", "mp3":
			return "media"
		default:
			return "other"
		}
	}
}

type builder struct {
	readyMsgs              []Message
	timestamp              uint64
	lastProcessedTimestamp int64
	peBuilder              *pageEventBuilder // TODO: DB
	ptaBuilder             *performanceTrackAggrBuilder
	ieBuilder              *inputEventBuilder // TODO: DB
	ciFinder               *cpuIssueFinder
	miFinder               *memoryIssueFinder
	ddDetector             *domDropDetector
	crDetector             *clickRageDetector
	dcDetector             *deadClickDetector
	integrationsWaiting    bool

	sid uint64
}

func NewBuilder() *builder {
	return &builder{
		peBuilder:           &pageEventBuilder{},
		ptaBuilder:          &performanceTrackAggrBuilder{},
		ieBuilder:           NewInputEventBuilder(),
		ciFinder:            &cpuIssueFinder{},
		miFinder:            &memoryIssueFinder{},
		ddDetector:          &domDropDetector{},
		crDetector:          &clickRageDetector{},
		dcDetector:          &deadClickDetector{},
		integrationsWaiting: true,
	}
}

func (b *builder) appendReadyMessage(msg Message) { // interface is never nil even if it holds nil value
	b.readyMsgs = append(b.readyMsgs, msg)
}

func (b *builder) iterateReadyMessage(iter func(msg Message)) {
	for _, readyMsg := range b.readyMsgs {
		iter(readyMsg)
	}
	b.readyMsgs = nil
}

func (b *builder) buildSessionEnd() {
	if b.timestamp == 0 {
		return
	}
	sessionEnd := &SessionEnd{
		Timestamp: b.timestamp, // + delay?
	}
	b.appendReadyMessage(sessionEnd)
}

func (b *builder) buildPageEvent() {
	if msg := b.peBuilder.Build(); msg != nil {
		b.appendReadyMessage(msg)
	}
}
func (b *builder) buildPerformanceTrackAggr() {
	if msg := b.ptaBuilder.Build(); msg != nil {
		b.appendReadyMessage(msg)
	}
}
func (b *builder) buildInputEvent() {
	if msg := b.ieBuilder.Build(); msg != nil {
		b.appendReadyMessage(msg)
	}
}

func (b *builder) handleMessage(message Message, messageID uint64) {
	timestamp := GetTimestamp(message)
	if b.timestamp < timestamp { // unnecessary? TODO: test and remove
		b.timestamp = timestamp
	}

	b.lastProcessedTimestamp = time.Now().UnixMilli()

	// Might happen before  the first timestamp.
	// TODO: to DB
	switch msg := message.(type) {
	case *SessionStart,
		*Metadata,
		*UserID,
		*UserAnonymousID:
		b.appendReadyMessage(msg)
	case *RawErrorEvent:
		b.appendReadyMessage(&ErrorEvent{
			MessageID: messageID,
			Timestamp: msg.Timestamp,
			Source:    msg.Source,
			Name:      msg.Name,
			Message:   msg.Message,
			Payload:   msg.Payload,
		})
	}
	if b.timestamp == 0 {
		return
	}
	switch msg := message.(type) {
	case *SetPageLocation: // TODO: DB
		if msg.NavigationStart == 0 {
			b.appendReadyMessage(&PageEvent{
				URL:       msg.URL,
				Referrer:  msg.Referrer,
				Loaded:    false,
				MessageID: messageID,
				Timestamp: b.timestamp,
			})
		} else {
			b.buildPageEvent()
			b.buildInputEvent()
			b.ieBuilder.ClearLabels()
			b.peBuilder.HandleSetPageLocation(msg, messageID, b.timestamp)
			b.miFinder.HandleSetPageLocation(msg)
			b.ciFinder.HandleSetPageLocation(msg)
		}
	case *PageLoadTiming: // TODO: DB
		if rm := b.peBuilder.HandlePageLoadTiming(msg); rm != nil {
			b.appendReadyMessage(rm)
		}
	case *PageRenderTiming: // TODO: DB
		if rm := b.peBuilder.HandlePageRenderTiming(msg); rm != nil {
			b.appendReadyMessage(rm)
		}
	case *PerformanceTrack:
		if rm := b.ptaBuilder.HandlePerformanceTrack(msg, b.timestamp); rm != nil {
			b.appendReadyMessage(rm)
		}
		if rm := b.ciFinder.HandlePerformanceTrack(msg, messageID, b.timestamp); rm != nil {
			b.appendReadyMessage(rm)
		}
		if rm := b.miFinder.HandlePerformanceTrack(msg, messageID, b.timestamp); rm != nil {
			b.appendReadyMessage(rm)
		}
	case *SetInputTarget: // TODO: DB
		if rm := b.ieBuilder.HandleSetInputTarget(msg); rm != nil {
			b.appendReadyMessage(rm)
		}
	case *SetInputValue: // TODO: DB
		if rm := b.ieBuilder.HandleSetInputValue(msg, messageID, b.timestamp); rm != nil {
			b.appendReadyMessage(rm)
		}
	case *MouseClick: // TODO: DB
		b.buildInputEvent()
		if rm := b.crDetector.HandleMouseClick(msg, messageID, b.timestamp); rm != nil {
			b.appendReadyMessage(rm)
		}
		if msg.Label != "" { // TODO: DB
			b.appendReadyMessage(&ClickEvent{
				MessageID:      messageID,
				Label:          msg.Label,
				HesitationTime: msg.HesitationTime,
				Timestamp:      b.timestamp,
				Selector:       msg.Selector,
			})
		}
	case *JSException:
		b.appendReadyMessage(&ErrorEvent{ // TODO: DB
			MessageID: messageID,
			Timestamp: b.timestamp,
			Source:    "js_exception",
			Name:      msg.Name,
			Message:   msg.Message,
			Payload:   msg.Payload,
		})
	case *ResourceTiming:
		tp := getResourceType(msg.Initiator, msg.URL)
		success := msg.Duration != 0
		b.appendReadyMessage(&ResourceEvent{ // TODO: DB
			MessageID:       messageID,
			Timestamp:       msg.Timestamp,
			Duration:        msg.Duration,
			TTFB:            msg.TTFB,
			HeaderSize:      msg.HeaderSize,
			EncodedBodySize: msg.EncodedBodySize,
			DecodedBodySize: msg.DecodedBodySize,
			URL:             msg.URL,
			Type:            tp,
			Success:         success,
		})
		if !success {
			issueType := "missing_resource"
			if tp == "fetch" {
				issueType = "bad_request"
			}
			b.appendReadyMessage(&IssueEvent{
				Type:          issueType,
				MessageID:     messageID,
				Timestamp:     msg.Timestamp,
				ContextString: msg.URL,
			})
		}
	case *RawCustomEvent:
		b.appendReadyMessage(&CustomEvent{ // TODO: DB
			MessageID: messageID,
			Timestamp: b.timestamp,
			Name:      msg.Name,
			Payload:   msg.Payload,
		})
	case *CustomIssue:
		b.appendReadyMessage(&IssueEvent{ // TODO: DB
			Type:          "custom",
			Timestamp:     b.timestamp,
			MessageID:     messageID,
			ContextString: msg.Name,
			Payload:       msg.Payload,
		})
	case *Fetch:
		b.appendReadyMessage(&FetchEvent{ // TODO: DB
			MessageID: messageID,
			Timestamp: msg.Timestamp,
			Method:    msg.Method,
			URL:       msg.URL,
			Request:   msg.Request,
			Response:  msg.Response,
			Status:    msg.Status,
			Duration:  msg.Duration,
		})
		if msg.Status >= 400 {
			b.appendReadyMessage(&IssueEvent{
				Type:          "bad_request",
				MessageID:     messageID,
				Timestamp:     msg.Timestamp,
				ContextString: msg.URL,
			})
		}
	case *GraphQL:
		b.appendReadyMessage(&GraphQLEvent{ // TODO: DB
			MessageID:     messageID,
			Timestamp:     b.timestamp,
			OperationKind: msg.OperationKind,
			OperationName: msg.OperationName,
			Variables:     msg.Variables,
			Response:      msg.Response,
		})
	case *StateAction:
		b.appendReadyMessage(&StateActionEvent{ // TODO: DB
			MessageID: messageID,
			Timestamp: b.timestamp,
			Type:      msg.Type,
		})
	case *CreateElementNode,
		*CreateTextNode:
		b.ddDetector.HandleNodeCreation()
	case *RemoveNode:
		b.ddDetector.HandleNodeRemoval(b.timestamp)
	case *CreateDocument:
		if rm := b.ddDetector.Build(); rm != nil {
			b.appendReadyMessage(rm)
		}
	}
	if rm := b.dcDetector.HandleMessage(message, messageID, b.timestamp); rm != nil {
		b.appendReadyMessage(rm)
	}
}

func (b *builder) checkTimeouts(ts int64) bool {
	if b.timestamp == 0 {
		return false // There was no timestamp events yet
	}

	if b.peBuilder.HasInstance() && int64(b.peBuilder.GetTimestamp())+intervals.EVENTS_PAGE_EVENT_TIMEOUT < ts {
		b.buildPageEvent()
	}
	if b.ieBuilder.HasInstance() && int64(b.ieBuilder.GetTimestamp())+intervals.EVENTS_INPUT_EVENT_TIMEOUT < ts {
		b.buildInputEvent()
	}
	if b.ptaBuilder.HasInstance() && int64(b.ptaBuilder.GetStartTimestamp())+intervals.EVENTS_PERFORMANCE_AGGREGATION_TIMEOUT < ts {
		b.buildPerformanceTrackAggr()
	}

	lastTsGap := ts - int64(b.timestamp)
	//b.lastProcessedTimestamp
	//log.Printf("checking timeouts for sess %v: %v now, %v sesstime; gap %v",b.sid,  ts, b.timestamp, lastTsGap)
	if lastTsGap > intervals.EVENTS_SESSION_END_TIMEOUT {
		if rm := b.ddDetector.Build(); rm != nil {
			b.appendReadyMessage(rm)
		}
		if rm := b.ciFinder.Build(); rm != nil {
			b.appendReadyMessage(rm)
		}
		if rm := b.miFinder.Build(); rm != nil {
			b.appendReadyMessage(rm)
		}
		if rm := b.crDetector.Build(); rm != nil {
			b.appendReadyMessage(rm)
		}
		if rm := b.dcDetector.HandleReaction(b.timestamp); rm != nil {
			b.appendReadyMessage(rm)
		}
		b.buildSessionEnd()
		return true
	}
	return false
}