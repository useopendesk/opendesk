defmodule RealtimeWeb.JoinTest do
  use RealtimeWeb.ChannelCase, async: true

  alias RealtimeWeb.{ContactChannel, TeamChannel, TicketChannel, UserChannel, WorkspaceChannel}

  @forbidden {:error, %{reason: "forbidden"}}

  setup do
    agent = Tokens.ws_claims()
    {:ok, agent_socket} = connect_claims(agent)

    {:ok, key_socket} =
      connect_claims(Tokens.ws_claims(%{"kind" => "api_key", "wid" => agent["wid"]}))

    contact = Tokens.widget_claims(%{"wid" => agent["wid"]})
    {:ok, contact_socket} = connect_claims(contact)

    %{
      wid: agent["wid"],
      uid: agent["sub"],
      cid: contact["sub"],
      agent: agent_socket,
      key: key_socket,
      contact: contact_socket
    }
  end

  describe "workspace:<wid>" do
    test "users and API keys of that workspace join", c do
      assert {:ok, %{server_time: t}, _} =
               subscribe_and_join(c.agent, WorkspaceChannel, "workspace:#{c.wid}")

      assert {:ok, _, _} = DateTime.from_iso8601(t)
      assert {:ok, _, _} = subscribe_and_join(c.key, WorkspaceChannel, "workspace:#{c.wid}")
    end

    test "another workspace, a contact and malformed ids are forbidden", c do
      for {socket, topic} <- [
            {c.agent, "workspace:#{Tokens.uuid()}"},
            {c.contact, "workspace:#{c.wid}"},
            {c.agent, "workspace:"},
            {c.agent, "workspace:#{String.upcase(c.wid)}"}
          ] do
        assert subscribe_and_join(socket, WorkspaceChannel, topic) == @forbidden, topic
      end
    end
  end

  describe "team:<wid>:<tid>" do
    setup c do
      team = Tokens.uuid()

      {:ok, member} =
        connect_claims(Tokens.ws_claims(%{"wid" => c.wid, "teams" => [String.upcase(team)]}))

      {:ok, key} =
        connect_claims(
          Tokens.ws_claims(%{"wid" => c.wid, "kind" => "api_key", "teams" => [team]})
        )

      %{team: team, member: member, granted_key: key}
    end

    test "users and API keys whose teams claim lists the team join", c do
      assert {:ok, _, _} = subscribe_and_join(c.member, TeamChannel, "team:#{c.wid}:#{c.team}")

      assert {:ok, _, _} =
               subscribe_and_join(c.granted_key, TeamChannel, "team:#{c.wid}:#{c.team}")
    end

    test "sockets without the team, other teams and workspaces, contacts and malformed ids are forbidden",
         c do
      other_wid = Tokens.uuid()

      {:ok, elsewhere} =
        connect_claims(Tokens.ws_claims(%{"wid" => other_wid, "teams" => [c.team]}))

      for {socket, topic} <- [
            {c.agent, "team:#{c.wid}:#{c.team}"},
            {c.key, "team:#{c.wid}:#{c.team}"},
            {c.contact, "team:#{c.wid}:#{c.team}"},
            {c.member, "team:#{c.wid}:#{Tokens.uuid()}"},
            {c.member, "team:#{other_wid}:#{c.team}"},
            {elsewhere, "team:#{c.wid}:#{c.team}"},
            {c.member, "team:#{c.wid}:#{String.upcase(c.team)}"},
            {c.member, "team:#{c.wid}"},
            {c.member, "team:#{c.team}"},
            {c.member, "team:#{c.wid}:#{c.team}:extra"}
          ] do
        assert subscribe_and_join(socket, TeamChannel, topic) == @forbidden, topic
      end
    end
  end

  describe "user:<uid>" do
    test "the user themself joins", c do
      assert {:ok, _, _} = subscribe_and_join(c.agent, UserChannel, "user:#{c.uid}")
    end

    test "another user, an API key and a contact are forbidden", c do
      assert subscribe_and_join(c.agent, UserChannel, "user:#{Tokens.uuid()}") == @forbidden
      assert subscribe_and_join(c.key, UserChannel, "user:#{c.key.assigns.sub}") == @forbidden
      assert subscribe_and_join(c.contact, UserChannel, "user:#{c.cid}") == @forbidden
    end
  end

  describe "ticket:<wid>:<ticket_id>" do
    test "users of the workspace join", c do
      assert {:ok, _, _} =
               subscribe_and_join(c.agent, TicketChannel, "ticket:#{c.wid}:#{Tokens.uuid()}")
    end

    test "other workspaces, API keys, contacts and malformed ids are forbidden", c do
      tid = Tokens.uuid()

      for {socket, topic} <- [
            {c.agent, "ticket:#{Tokens.uuid()}:#{tid}"},
            {c.key, "ticket:#{c.wid}:#{tid}"},
            {c.contact, "ticket:#{c.wid}:#{tid}"},
            {c.agent, "ticket:#{c.wid}:not-a-uuid"},
            {c.agent, "ticket:#{c.wid}:#{String.upcase(tid)}"},
            {c.agent, "ticket:#{c.wid}"},
            {c.agent, "ticket:#{c.wid}:#{tid}:extra"}
          ] do
        assert subscribe_and_join(socket, TicketChannel, topic) == @forbidden, topic
      end
    end
  end

  describe "contact:<wid>:<contact_id>" do
    test "the contact joins its own topic", c do
      assert {:ok, _, _} =
               subscribe_and_join(c.contact, ContactChannel, "contact:#{c.wid}:#{c.cid}")
    end

    test "another contact, another workspace and agents are forbidden", c do
      for {socket, topic} <- [
            {c.contact, "contact:#{c.wid}:#{Tokens.uuid()}"},
            {c.contact, "contact:#{Tokens.uuid()}:#{c.cid}"},
            {c.agent, "contact:#{c.wid}:#{c.cid}"},
            {c.key, "contact:#{c.wid}:#{c.cid}"},
            {c.contact, "contact:#{c.wid}:#{c.cid}:x"}
          ] do
        assert subscribe_and_join(socket, ContactChannel, topic) == @forbidden, topic
      end
    end
  end

  test "topics without a channel are not routed", c do
    assert_raise RuntimeError, ~r/no channel found/, fn ->
      subscribe_and_join(c.agent, "admin:#{c.wid}")
    end
  end
end
