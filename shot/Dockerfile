FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/shot .

# Headless Chrome, plus CJK and emoji fonts so Chinese pages look right.
FROM chromedp/headless-shell:latest
RUN apt-get update \
 && apt-get install -y --no-install-recommends fontconfig fonts-noto-cjk fonts-noto-color-emoji \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 1001 --create-home shot
COPY --from=build /out/shot /usr/local/bin/shot
USER shot
ENV CHROME_PATH=/headless-shell/headless-shell LISTEN=:9000
EXPOSE 9000
ENTRYPOINT ["/usr/local/bin/shot"]
