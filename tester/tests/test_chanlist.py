import requests


def test_equals():
    assert 1 == 1


def test_get_channels(base_url):
    r = requests.get(f'http://{base_url}/channels.m3u')
    r.raise_for_status()
    assert r.text == 'hai'
