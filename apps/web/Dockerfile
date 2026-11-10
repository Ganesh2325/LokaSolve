FROM node:22-bookworm-slim
WORKDIR /app
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/package.json
RUN npm ci
COPY apps/web apps/web
WORKDIR /app/apps/web
ARG API_PROXY_TARGET=http://gateway:8088
ENV API_PROXY_TARGET=$API_PROXY_TARGET
RUN npm run build
EXPOSE 3000
CMD ["npm", "run", "start"]
