#!/usr/bin/env python3
"""
SeaweedFS Bucket Info Script
Creates a bucket, uploads test files, and retrieves bucket object count and size.
Uses async/parallel fetching for all buckets.
"""

import boto3
import asyncio
import aiobotocore.session
import os
import uuid
import sys
from datetime import datetime
from concurrent.futures import ThreadPoolExecutor

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


def list_all_buckets(s3_client):
    """List all buckets."""
    response = s3_client.list_buckets()
    return [bucket["Name"] for bucket in response.get("Buckets", [])]


async def get_bucket_info_async(bucket_name, session):
    """Get bucket object count and total size using async S3 API."""
    total_size = 0
    object_count = 0
    
    config = aiobotocore.config.AioConfig(
        endpoint_url=S3_ENDPOINT,
        aws_access_key_id=S3_ACCESS_KEY,
        aws_secret_access_key=S3_SECRET_KEY,
    )
    
    async with session.create_client("s3", config=config) as client:
        try:
            paginator = client.get_paginator("list_objects_v2")
            async for page in paginator.paginate(Bucket=bucket_name):
                if "Contents" in page:
                    for obj in page["Contents"]:
                        total_size += obj.get("Size", 0)
                        object_count += 1
            return bucket_name, object_count, total_size
        except Exception as e:
            print(f"Error getting bucket info for {bucket_name}: {e}")
            return bucket_name, None, None


async def get_all_buckets_info_async(bucket_names):
    """Fetch bucket info for all buckets in parallel."""
    session = aiobotocore.session.get_session()
    tasks = [get_bucket_info_async(name, session) for name in bucket_names]
    results = await asyncio.gather(*tasks)
    return results


def get_bucket_info_sync(s3_client, bucket_name):
    """Get bucket info synchronously (fallback)."""
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


def get_all_buckets_info_parallel(bucket_names, max_workers=10):
    """Fetch bucket info for all buckets in parallel using thread pool."""
    s3_client = create_s3_client()
    
    with ThreadPoolExecutor(max_workers=max_workers) as executor:
        futures = {
            executor.submit(get_bucket_info_sync, s3_client, name): name 
            for name in bucket_names
        }
        
        results = []
        for future in futures:
            bucket_name = futures[future]
            try:
                object_count, total_size = future.result()
                results.append((bucket_name, object_count, total_size))
            except Exception as e:
                print(f"Error for bucket {bucket_name}: {e}")
                results.append((bucket_name, None, None))
    
    return results


def get_bucket_info_via_filer(filer_url="http://localhost:8888"):
    """Get bucket info via Filer REST API."""
    import requests
    
    try:
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
    
    create_bucket(s3_client, BUCKET_NAME)
    
    print("\nUploading test files...")
    upload_test_files(s3_client, BUCKET_NAME, count=5)
    
    print("\nFetching all buckets...")
    buckets = list_all_buckets(s3_client)
    print(f"Found {len(buckets)} buckets: {buckets}")
    
    print("\nFetching bucket info in parallel (async)...")
    try:
        results = asyncio.run(get_all_buckets_info_async(buckets))
    except Exception as e:
        print(f"Async failed, using thread pool: {e}")
        results = get_all_buckets_info_parallel(buckets)
    
    print(f"\n{'=' * 60}")
    print("Bucket Information (Parallel Fetch)")
    print(f"{'=' * 60}")
    print(f"{'Bucket Name':<30} {'Objects':<10} {'Size (bytes)':<15}")
    print("-" * 60)
    
    total_objects = 0
    total_size = 0
    
    for bucket_name, object_count, size in results:
        if object_count is not None:
            print(f"{bucket_name:<30} {object_count:<10} {size:<15}")
            total_objects += object_count
            total_size += size
    
    print("-" * 60)
    print(f"{'TOTAL':<30} {total_objects:<10} {total_size:<15}")
    print(f"{'=' * 60}")
    
    get_bucket_info_via_filer()
    
    print("\nDone!")


if __name__ == "__main__":
    main()
