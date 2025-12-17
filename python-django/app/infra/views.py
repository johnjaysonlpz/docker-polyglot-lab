import logging
from django.http import HttpResponse, JsonResponse
from django_app import settings
from . import readiness
from . import metrics as metrics_module

log = logging.getLogger("infra")

def root(request):
    return HttpResponse("python-django-app is running (Python + Django)\n", content_type="text/plain")

def info(request):
    return JsonResponse(
        {
            "service": settings.APP_SERVICE_NAME,
            "version": settings.APP_VERSION,
            "buildTime": settings.APP_BUILD_TIME,
        }
    )

def health(request):
    return HttpResponse(status=200)

def ready(request):
    if readiness.state.is_accepting():
        return HttpResponse(status=200)
    return HttpResponse(status=503)

def metrics_view(request):
    body, content_type = metrics_module.scrape_metrics()
    return HttpResponse(body, content_type=content_type)

metrics = metrics_view
