from __future__ import annotations

import functools
from collections.abc import Callable

from django.conf import settings
from django.http import HttpRequest, HttpResponse, JsonResponse

from . import errors, metrics as metrics_module, readiness


def require_get(fn: Callable[[HttpRequest], HttpResponse]) -> Callable[[HttpRequest], HttpResponse]:
    @functools.wraps(fn)
    def wrapped(request: HttpRequest, *args: object, **kwargs: object) -> HttpResponse:
        if request.method != "GET":
            return errors.method_not_allowed(request, allow="GET")
        return fn(request)

    return wrapped


@require_get
def root(_request: HttpRequest) -> HttpResponse:
    return HttpResponse(
        "python-django-app is running (Python + Django)\n",
        content_type="text/plain",
    )


@require_get
def info(_request: HttpRequest) -> JsonResponse:
    return JsonResponse(
        {
            "service": settings.SERVICE_NAME,
            "version": settings.VERSION,
            "build_time": settings.BUILD_TIME,
        }
    )


@require_get
def health(_request: HttpRequest) -> HttpResponse:
    return HttpResponse(status=200)


@require_get
def ready(_request: HttpRequest) -> HttpResponse:
    return HttpResponse(status=200 if readiness.state.is_accepting() else 503)


@require_get
def metrics_view(_request: HttpRequest) -> HttpResponse:
    body, content_type = metrics_module.scrape_metrics()
    return HttpResponse(body, content_type=content_type)


metrics = metrics_view
