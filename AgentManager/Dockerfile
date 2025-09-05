FROM python:3.13-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    gcc \
    g++ \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy source code
COPY src/ src/
COPY proto/ proto/
COPY generate_grpc.sh .

# Generate gRPC code
RUN chmod +x generate_grpc.sh && ./generate_grpc.sh

# Set Python path
ENV PYTHONPATH=/app

# Default command
CMD ["python", "-m", "src.main"]
