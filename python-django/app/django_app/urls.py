from django.urls import path

from infra import views

urlpatterns = [
    path("", views.root, name="root"),
    path("info", views.info, name="info"),
    path("health", views.health, name="health"),
    path("ready", views.ready, name="ready"),
    path("metrics", views.metrics_view, name="metrics"),
]

handler400 = "infra.errors.handler400"
handler403 = "infra.errors.handler403"
handler404 = "infra.errors.handler404"
handler500 = "infra.errors.handler500"
