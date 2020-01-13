import pytest
import os


@pytest.fixture()
def base_url():
    return os.getenv('BASE_URL', 'localhost:8080')
