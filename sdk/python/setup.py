from setuptools import setup, find_packages

setup(
    name="mirage-sdk",
    version="1.0.0",
    packages=find_packages(),
    install_requires=[
        "grpcio>=1.50.0",
        "grpcio-tools>=1.50.0",
        "websocket-client>=1.4.0",
    ],
    python_requires=">=3.8",
)
