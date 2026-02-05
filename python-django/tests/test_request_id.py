from infra.request_id import get_request_id, reset_request_id, set_request_id


def test_request_id_contextvar_roundtrip() -> None:
    assert get_request_id() in ("-", "")

    token = set_request_id("abc")
    try:
        assert get_request_id() == "abc"
    finally:
        reset_request_id(token)

    assert get_request_id() in ("-", "")
    token2 = set_request_id("")
    try:
        assert get_request_id() == "-"
    finally:
        reset_request_id(token2)
