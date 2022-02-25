FROM registry.ci.openshift.org/openshift/release:golang-1.17 AS builder
USER 0
ENV GOPATH /go
WORKDIR /go/src/github.com/vrutkovs/slack-jira-bot
COPY . .
RUN go build -mod=mod ./cmd/slack-jira-bot/

FROM registry.access.redhat.com/ubi8/ubi-minimal:8.5
COPY --from=builder /go/src/github.com/vrutkovs/slack-jira-bot/slack-jira-bot /bin/slack-jira-bot
WORKDIR /srv
ENTRYPOINT ["/bin/slack-jira-bot"]
