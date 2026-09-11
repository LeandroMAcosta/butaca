FROM python:3.12-slim

# ffmpeg supplies ffprobe, which reads the audio and subtitle tracks of a file,
# and the audio that ffsubsync syncs subtitles against.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg curl ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# alass is the fallback subtitle syncer. Upstream publishes a static binary
# for x86-64 only (last release 2.0.0, 2020); elsewhere the sidecar runs
# without it and says so in each sync report.
ARG TARGETARCH
RUN if [ "$TARGETARCH" = "amd64" ] || [ "$(uname -m)" = "x86_64" ]; then \
      curl -fsSL -o /usr/local/bin/alass-cli \
        https://github.com/kaegi/alass/releases/download/v2.0.0/alass-linux64 \
   && echo "7bd0b9ae7e035d3ba940eacffb21243614df36231d47f21f0b4ce42001ab7fcd  /usr/local/bin/alass-cli" | sha256sum -c - \
   && chmod +x /usr/local/bin/alass-cli; \
    fi

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY *.py .

EXPOSE 8000
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8000"]
