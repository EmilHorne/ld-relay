package metrics

import (
	"testing"
	"time"

	"github.com/launchdarkly/ld-relay/v6/config"
	st "github.com/launchdarkly/ld-relay/v6/internal/core/sharedtest"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

type args struct {
	measure   Measure
	platform  string
	userAgent string
}

func (a args) getExpectedTagsMap() map[string]string {
	return map[string]string{
		platformCategoryTagKey.Name(): a.platform,
		userAgentTagKey.Name():        a.userAgent,
	}
}

func TestAddEnvironmentWithoutEventPublisher(t *testing.T) {
	manager, err := NewManager(config.MetricsConfig{}, 0, ldlog.NewDisabledLoggers())
	require.NoError(t, err)
	defer manager.Close()

	env, err := manager.AddEnvironment("name", nil)

	assert.NoError(t, err)
	require.NotNil(t, env)
	assert.NotNil(t, env.GetOpenCensusContext())
}

func TestAddEnvironmentWithEventPublisher(t *testing.T) {
	publisher := newTestEventsPublisher()
	view.SetReportingPeriod(testReportingPeriod)
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})

	manager, err := NewManager(config.MetricsConfig{}, 0, ldlog.NewDisabledLoggers())
	require.NoError(t, err)
	defer manager.Close()

	env, err := manager.AddEnvironment("name", publisher)

	assert.NoError(t, err)
	require.NotNil(t, env)
	assert.NotNil(t, env.GetOpenCensusContext())

	stats.Record(env.GetOpenCensusContext(), privateConnMeasure.M(1))

	require.Eventually(t, func() bool {
		env.FlushEventsExporter()
		select {
		case event := <-publisher.events:
			require.IsType(t, relayMetricsEvent{}, event)
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond*10)
}

func TestAddEnvironmentAfterManagerClosed(t *testing.T) {
	manager, err := NewManager(config.MetricsConfig{}, 0, ldlog.NewDisabledLoggers())
	require.NoError(t, err)
	manager.Close()
	env, err := manager.AddEnvironment("name", nil)
	assert.Nil(t, env)
	assert.Error(t, err)
}

func TestRemoveEnvironment(t *testing.T) {
	manager, err := NewManager(config.MetricsConfig{}, 0, ldlog.NewDisabledLoggers())
	require.NoError(t, err)
	defer manager.Close()

	env, err := manager.AddEnvironment("name", nil)
	require.NoError(t, err)
	require.NotNil(t, env)

	manager.RemoveEnvironment(env)

	manager.lock.Lock()
	defer manager.lock.Unlock()
	assert.Len(t, manager.environments, 0)
}

func TestConnectionMetrics(t *testing.T) {
	specs := []args{
		args{platform: browserTagValue, measure: BrowserConns, userAgent: userAgentValue},
		args{platform: mobileTagValue, measure: MobileConns, userAgent: userAgentValue},
		args{platform: serverTagValue, measure: ServerConns, userAgent: userAgentValue},
	}

	for _, tt := range specs {
		t.Run(tt.platform, func(*testing.T) {
			testWithExporter(t, func(p testWithExporterParams) {
				expectedTags := tt.getExpectedTagsMap()
				expectedTags[envNameTagKey.Name()] = p.envName
				expectedPrivateTags := tt.getExpectedTagsMap()
				expectedPrivateTags[relayIDTagKey.Name()] = p.relayID
				expectedPrivateTags[envNameTagKey.Name()] = p.envName

				WithGauge(p.env.GetOpenCensusContext(), userAgentValue, func() {
					p.exporter.AwaitData(t, time.Second, p.mockLog.Loggers, func(d st.TestMetricsData) bool {
						return d.HasRow(publicConnView.Name, st.TestMetricsRow{
							Tags: expectedTags,
							Sum:  1,
						}) && d.HasRow(privateConnView.Name, st.TestMetricsRow{
							Tags: expectedPrivateTags,
							Sum:  1,
						})
					})
				}, tt.measure)

				p.exporter.AwaitData(t, time.Second, p.mockLog.Loggers, func(d st.TestMetricsData) bool {
					return d.HasRow(publicConnView.Name, st.TestMetricsRow{
						Tags: expectedTags,
						Sum:  0,
					}) && d.HasRow(privateConnView.Name, st.TestMetricsRow{
						Tags: expectedPrivateTags,
						Sum:  0,
					})
				})
			})
		})
	}
}

func TestNewConnectionMetrics(t *testing.T) {
	specs := []args{
		args{platform: browserTagValue, measure: NewBrowserConns, userAgent: userAgentValue},
		args{platform: mobileTagValue, measure: NewMobileConns, userAgent: userAgentValue},
		args{platform: serverTagValue, measure: NewServerConns, userAgent: userAgentValue},
	}

	for _, tt := range specs {
		t.Run(tt.platform, func(*testing.T) {
			testWithExporter(t, func(p testWithExporterParams) {
				expectedTags := tt.getExpectedTagsMap()
				expectedTags[envNameTagKey.Name()] = p.envName
				expectedPrivateTags := tt.getExpectedTagsMap()
				expectedPrivateTags[relayIDTagKey.Name()] = p.relayID
				expectedPrivateTags[envNameTagKey.Name()] = p.envName
				WithCount(p.env.GetOpenCensusContext(), userAgentValue, func() {}, tt.measure)
				p.exporter.AwaitData(t, time.Second, p.mockLog.Loggers, func(d st.TestMetricsData) bool {
					return d.HasRow(publicNewConnView.Name, st.TestMetricsRow{
						Tags: expectedTags,
						Sum:  1,
					}) && d.HasRow(privateNewConnView.Name, st.TestMetricsRow{
						Tags: expectedPrivateTags,
						Sum:  1,
					})
				})
			})
		})
	}
}

func TestWithRouteCount(t *testing.T) {
	testWithExporter(t, func(p testWithExporterParams) {
		WithRouteCount(p.env.GetOpenCensusContext(), userAgentValue, "someRoute", "GET", func() {
			p.exporter.AwaitData(t, time.Second, p.mockLog.Loggers, func(d st.TestMetricsData) bool {
				return d.HasRow(requestView.Name, st.TestMetricsRow{
					Tags: map[string]string{
						"env":              p.envName,
						"method":           "GET",
						"platformCategory": "server",
						"route":            "someRoute",
						"userAgent":        userAgentValue,
					},
					Count: 1,
				})
			})
		}, ServerRequests)
		sp := p.exporter.AwaitSpan(t, time.Second)
		assert.Equal(t, "someRoute", sp.Name)
	})
}

func TestSanitizeTagValue(t *testing.T) {
	assert.Equal(t, "abc", sanitizeTagValue("abc"))
	assert.Equal(t, "_", sanitizeTagValue(""))
}
