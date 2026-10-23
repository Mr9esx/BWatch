"""Gate XHS API attempts made by the yt-dlp Python subprocess."""

import http.client
import os

from xhs_rate_limiter import acquire_xhs_token, is_xhs_api_url


if os.environ.get("BILIFETCH_XHS_YTDLP_LIMIT") == "1":
    _http_request = http.client.HTTPConnection.request

    def _limited_http_request(self, method, url, *args, **kwargs):
        target = str(url)
        if not target.startswith(("http://", "https://")):
            target = f"https://{self.host}{target}"
        if is_xhs_api_url(target):
            acquire_xhs_token()
        return _http_request(self, method, url, *args, **kwargs)

    http.client.HTTPConnection.request = _limited_http_request

    try:
        import urllib3.connection

        _urllib3_request = urllib3.connection.HTTPConnection.request

        def _limited_urllib3_request(self, method, url, *args, **kwargs):
            target = str(url)
            if not target.startswith(("http://", "https://")):
                target = f"https://{self.host}{target}"
            if is_xhs_api_url(target):
                acquire_xhs_token()
            return _urllib3_request(self, method, url, *args, **kwargs)

        urllib3.connection.HTTPConnection.request = _limited_urllib3_request
    except ImportError:
        pass

    try:
        from curl_cffi import requests as curl_requests

        _curl_request = curl_requests.Session.request

        def _limited_curl_request(self, method, url, *args, **kwargs):
            if is_xhs_api_url(str(url)):
                acquire_xhs_token()
            return _curl_request(self, method, url, *args, **kwargs)

        curl_requests.Session.request = _limited_curl_request
    except ImportError:
        pass
