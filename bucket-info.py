#!/usr/bin/env python3
"""
SeaweedFS Bucket Info Script
Creates a bucket, uploads test files, and retrieves bucket object count and size.
"""

import boto3
import os
import uuid
import sys
from datetime import datetime

S3_ENDPOINT = "http://localhost:8333"
S3_ACCESS_KEY = "admin"
S3_SECRET_KEY = "admin"
BUCKET_NAME = f"test-bucket-{uuid.uuid4().hex[:8]}"


def create_s3_client():
    return boto3.client(
        "s3",
        endpoint_url=S3_ENDPOINT,
        aws_access_key_id=S3_ACCESS_KEY,
        aws_secret_access_key=S3_SECRET_KEY,
    )


def create_bucket(s3_client, bucket_name):
    """Create an S3 bucket."""
    try:
        s3_client.create_bucket(Bucket=bucket_name)
        print(f"Created bucket: {bucket_name}")
    except Exception as e:
        print(f"Error creating bucket: {e}")
        sys.exit(1)


def upload_test_files(s3_client, bucket_name, count=5):
    """Upload test files to the bucket."""
    test_files = []
    for i in range(count):
        content = f"Test content for file {i}\nTimestamp: {datetime.now().isoformat()}"
        file_name = f"test-file-{i}.txt"
        file_path = f"/tmp/{file_name}"
        
        with open(file_path, "w") as f:
            f.write(content)
        
        s3_client.upload_file(file_path, bucket_name, file_name)
        test_files.append(file_name)
        print(f"Uploaded: {file_name}")
    
    return test_files


def get_bucket_info(s3_client, bucket_name):
    """Get bucket object count and total size using S3 API."""
    total_size = 0
    object_count = 0
    
    try:
        paginator = s3_client.get_paginator("list_objects_v2")
        page_iterator = paginator.paginate(Bucket=bucket_name)
        
        for page in page_iterator:
            if "Contents" in page:
                for obj in page["Contents"]:
                    total_size += obj.get("Size", 0)
                    object_count += 1
        
        return object_count, total_size
    except Exception as e:
        print(f"Error getting bucket info: {e}")
        return None, None


def get_bucket_info_via_filer(filer_url="http://localhost:8888"):
    """Get bucket info via Filer REST API."""
    import requests
    
    try:
        # List buckets via filer
        response = requests.get(f"{filer_url}/vol/buckets")
        if response.status_code == 200:
            buckets = response.json()
            print(f"\nBuckets via Filer API: {buckets}")
    except Exception as e:
        print(f"Filer API error: {e}")


def main():
    print("=" * 50)
    print("SeaweedFS Bucket Info Script")
    print("=" * 50)
    
    s3_client = create_s3_client()
    
    # Create bucket
    create_bucket(s3_client, BUCKET_NAME)
    
    # Upload test files
    print("\nUploading test files...")
    upload_test_files(s3_client, BUCKET_NAME, count=5)
    
    # Get bucket info via S3 API
    print("\nFetching bucket info via S3 API...")
    object_count, total_size = get_bucket_info(s3_client, BUCKET_NAME)
    
    print(f"\n{'=' * 50}")
    print("Bucket Information")
    print(f"{'=' * 50}")
    print(f"Bucket Name:     {BUCKET_NAME}")
    print(f"Object Count:    {object_count}")
    print(f"Total Size:      {total_size} bytes ({total_size / 1024:.2f} KB)")
    print(f"{'=' * 50}")
    
    # Also try via Filer API
    get_bucket_info_via_filer()
    
    print("\nDone!")


if __name__ == "__main__":
    main()
