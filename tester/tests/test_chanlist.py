import requests


def test_equals():
    assert 1 == 1


def test_get_channels(base_url):
    r = requests.get(f'http://{base_url}/c')
    r.raise_for_status()
    chans = [
        line
        for line in r.text.splitlines()
        if line.startswith(f'http://{base_url}/c/')
    ]
    assert len(chans) == 150
