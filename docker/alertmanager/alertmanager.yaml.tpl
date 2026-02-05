global:
  resolve_timeout: 5m

templates:
  - /etc/alertmanager/templates/*.tmpl

route:
  receiver: noop
  group_by: ["alertname", "job"]
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 2h
  __ROUTES__

receivers:
  - name: noop
__RECEIVERS__

inhibit_rules:
  - source_matchers: ['severity="critical"']
    target_matchers: ['severity=~"warning|info"']
    equal: ["alertname", "job"]

  - source_matchers: ['severity="warning"']
    target_matchers: ['severity="info"']
    equal: ["alertname", "job"]
