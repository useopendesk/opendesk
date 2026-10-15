defmodule RealtimeWeb.ContactBrandTest do
  @moduledoc "A visitor's widget socket receives only its own brand's chats."
  use RealtimeWeb.ChannelCase, async: true

  alias Phoenix.Socket.Message
  alias Realtime.Fanout
  alias RealtimeWeb.ContactChannel

  defp join_contact(wid, cid, brand) do
    claims = Tokens.widget_claims(%{"wid" => wid, "sub" => cid})
    claims = if brand, do: Map.put(claims, "bid", brand), else: claims
    {:ok, k} = connect_claims(claims)
    {:ok, _, _} = join(k, ContactChannel, "contact:#{wid}:#{cid}")
    :ok
  end

  defp envelope(wid, topics) do
    %{
      "id" => Tokens.uuid(),
      "seq" => System.unique_integer([:positive]),
      "workspace_id" => wid,
      "type" => "message.created",
      "visibility" => "public",
      "topics" => topics,
      "data" => %{"ticket_id" => Tokens.uuid(), "message" => %{"body" => "hi"}},
      "occurred_at" => "2026-10-11T09:00:00Z"
    }
  end

  defp live(wid, cid, brand) do
    %{
      "workspace_id" => wid,
      "ticket_id" => Tokens.uuid(),
      "contact_id" => cid,
      "brand_id" => brand,
      "type" => "ai.reply.delta",
      "data" => %{"reply_id" => Tokens.uuid(), "text" => "Hello"}
    }
  end

  setup do
    %{wid: Tokens.uuid(), cid: Tokens.uuid(), brand: Tokens.uuid(), other: Tokens.uuid()}
  end

  test "an event of the socket's brand is pushed without the brand", c do
    join_contact(c.wid, c.cid, c.brand)
    env = envelope(c.wid, ["contact:#{c.wid}:#{c.cid}:#{c.brand}"])
    contact = "contact:#{c.wid}:#{c.cid}"

    assert {:ok, ["workspace:" <> _, ^contact]} =
             Fanout.handle("od.events." <> c.wid, Jason.encode!(env))

    expected = Map.delete(env, "topics")
    assert_receive %Message{topic: ^contact, event: "event", payload: ^expected}
  end

  test "an event of another brand never reaches the socket", c do
    join_contact(c.wid, c.cid, c.brand)
    env = envelope(c.wid, ["contact:#{c.wid}:#{c.cid}:#{c.other}"])
    assert {:ok, _} = Fanout.handle("od.events." <> c.wid, Jason.encode!(env))
    refute_receive %Message{event: "event"}, 50
  end

  test "a live preview of another brand never reaches the socket", c do
    join_contact(c.wid, c.cid, c.brand)

    assert {:ok, _} =
             Fanout.handle("od.live." <> c.wid, Jason.encode!(live(c.wid, c.cid, c.other)))

    refute_receive %Message{event: "live"}, 50

    assert {:ok, _} =
             Fanout.handle("od.live." <> c.wid, Jason.encode!(live(c.wid, c.cid, c.brand)))

    assert_receive %Message{event: "live", payload: payload}
    refute Map.has_key?(payload, "brand_id")
  end

  test "a malformed brand in a live preview is dropped", c do
    assert Fanout.handle("od.live." <> c.wid, Jason.encode!(live(c.wid, c.cid, "nope"))) ==
             {:error, :invalid_live}
  end

  test "a contact topic with a malformed brand is ignored", c do
    env = envelope(c.wid, ["contact:#{c.wid}:#{c.cid}:nope"])

    assert Fanout.handle("od.events." <> c.wid, Jason.encode!(env)) ==
             {:ok, ["workspace:#{c.wid}"]}
  end
end
