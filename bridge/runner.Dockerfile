FROM node:24.18.0-slim
RUN npm install -g --no-fund --no-audit @anthropic-ai/claude-code@2.1.296 && npm cache clean --force
USER node
ENTRYPOINT ["claude"]
