FROM node:22-alpine

WORKDIR /app

# Install pnpm
RUN npm install -g pnpm

# Copy package files
COPY package.json pnpm-lock.yaml ./

# Install dependencies using pnpm
RUN pnpm install --frozen-lockfile

# Copy source code
COPY . .

# Expose Vite's default port
EXPOSE 5173

# Run Vite in dev mode using pnpm
CMD ["pnpm", "run", "dev", "--host", "0.0.0.0"]
