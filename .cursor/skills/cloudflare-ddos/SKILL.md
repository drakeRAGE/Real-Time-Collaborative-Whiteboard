---
name: cloudflare-ddos
description: >-
  Tune Cloudflare HTTP and network DDoS protection for this proxied whiteboard.
  Use when the user mentions DDoS, volumetric floods, L3/L4 or L7 attacks,
  adaptive DDoS, or DDoS override rules on the app.
---

# DDoS protection for this whiteboard

HTTP DDoS (phase `ddos_l7`) and network DDoS (phase `ddos_l4`) are on for proxied traffic. Fetch [DDoS protection](https://developers.cloudflare.com/ddos-protection/) and the [override rules](https://developers.cloudflare.com/ddos-protection/managed-rulesets/) page before a change. Availability of custom expressions, log action, and adaptive profiles depends on plan.

## Fit for this origin

- The public hostname on Render should be proxied. Grey-cloud DNS skips HTTP DDoS for that name.
- Socket.IO heartbeats and a classroom of cursors are bursty, not a volumetric flood. Do not set sensitivity to `eoff` to keep rooms online.
- WebRTC media does not pass through Cloudflare. A media-path flood is outside this zone. Signaling floods on `/socket.io/` are an HTTP DDoS and WAF rate-limit problem (`cloudflare-waf`).
- Network DDoS overrides are account-level. HTTP overrides may be zone or account. Zone overrides win over account overrides. Read both before editing.
- Most plans allow one override rule. Spend it on a named false positive, not a blanket disable.

## Change procedure

1. Confirm the event in DDoS analytics (action, ruleset, sensitivity, source). A WAF custom-rule block is not a DDoS override.
2. If a legitimate room was mitigated, add the narrowest override: one managed rule id or tag, on this zone, for the affected path when the plan allows a custom expression.
3. Use `log` only where the plan allows it, review, then restore `default` (high) sensitivity.
4. Adaptive DDoS profiles (origins, user-agents, locations, protocols) are for accounts that include them. A new whiteboard has no 7-day baseline. Do not treat the first week of adaptive alerts as an attack without looking at the series.

## Sensitivity

`default`, `medium`, `low`, `eoff`. Move one step toward `low` for a confirmed false positive. `eoff` means that rule is effectively off. Record the rule id and the event that justified it in the change description.
