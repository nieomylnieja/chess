#include <stdio.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <stdlib.h>
#include <unistd.h>
#include <string.h>
#include <errno.h>

#include "lib/log.c/src/log.h"

#define MAX_CLIENTS 100

#define RECONNECT_EVENT "RECONNECT"
#define BEGIN_EVENT "BEGIN"
#define END_EVENT "END"
#define WHITE_COLOR "WHITE"
#define BLACK_COLOR "BLACK"

int start_listening(int sfd);
int start_server(uint16_t port);

typedef struct {
    uint id;
    uint opponent_id;
    const char *color;
    int result;
    int fd;
    char response[256];
    int sent_ctr;
} client_t;

static const client_t empty_client_t;

client_t clients[MAX_CLIENTS];

int main(int argc, char *argv[]) {
    if (argc != 1 && argc != 3) {
        printf("please provide one of the below mentioned:\n"
               " - no args (default port: 1234)\n"
               " - port number\n");
        return 1;
    }
    uint16_t port = 1234;
    if (argc == 3) {
        char *end_ptr;
        port = strtol(argv[2], &end_ptr, 10);
        if (*end_ptr) {
            log_error("failed to convert: %s to int", argv[2]);
            return 1;
        }
    }

    int sfd = start_server(port);
    if (!sfd) {
        return 0;
    }
    log_info("starting to listen for incoming connections at port: %d", port);

    start_listening(sfd);

    close(sfd);
    return 0;
}

int start_server(uint16_t port) {
    int sfd, on = 1;

    sfd = socket(PF_INET, SOCK_STREAM, 0);
    if (sfd == -1) {
        log_error("failed to create socket: %s", strerror(errno));
        return 0;
    }
    if (setsockopt(sfd, SOL_SOCKET, SO_REUSEADDR, &on, sizeof(on)) == -1) {
        log_error("failed to set socket options: %s", strerror(errno));
        return 0;
    }

    struct sockaddr_in addr;
    addr.sin_family = PF_INET;
    addr.sin_port = htons(port);
    addr.sin_addr.s_addr = INADDR_ANY;

    if (bind(sfd, (const struct sockaddr *) &addr, sizeof(addr)) == -1) {
        log_error("failed to bind socket: %s", strerror(errno));
        return 0;
    }
    int max_conn = 100;
    if (listen(sfd, max_conn) == -1) {
        log_error("failed to start listening: %s", strerror(errno));
        return 0;
    }
    return sfd;
}

int start_listening(int sfd) {
    socklen_t sock_len;
    struct sockaddr_in caddr;
    int cfd, fd_count, fd_max;
    fd_set r_mask, w_mask, tmp_r_mask, tmp_w_mask;
    static struct timeval timeout;

    FD_ZERO(&r_mask);
    FD_ZERO(&w_mask);
    FD_ZERO(&tmp_r_mask);
    FD_ZERO(&tmp_w_mask);
    fd_max = sfd;

    /*/
     * Since a chess game is between two players and we're processing the traffic sequentially
     * we just assign the first unpaired client to the variable and then check it when a new client connects
     * Once the new client connects we can then pair them both and reset the variable
     */
    int awaiting = 0;

    while (1) {
        // set timeout for select
        // this has to done inside the loop since on linux select will modify the struct
        timeout.tv_sec = 5 * 60;
        timeout.tv_usec = 0;

        // set the sever fd first to allow new connections
        FD_SET(sfd, &r_mask);

        tmp_w_mask = w_mask;
        tmp_r_mask = r_mask;

        fd_count = select(MAX_CLIENTS + 1, &tmp_r_mask, &tmp_w_mask, NULL, &timeout);
        if (fd_count == -1) {
            log_error("error occurred while running select(): %s", strerror(errno));
            exit(1);
        }
        if (fd_count == 0) {
            log_warn("timed out");
            continue;
        }

        // check if we have a new connection ready
        if (FD_ISSET(sfd, &tmp_r_mask)) {
            fd_count -= 1;
            sock_len = sizeof(caddr);
            cfd = accept(sfd, (struct sockaddr *) &caddr, &sock_len);
            if (cfd == -1) {
                log_error("failed to extract connection request from queue: %s", strerror(errno));
                return 0;
            }
            log_info("new connection from: %s", inet_ntoa((struct in_addr) caddr.sin_addr));
            FD_SET(cfd, &r_mask);
            if (cfd > fd_max) {
                fd_max = cfd;
            }
        }

        // go through all of the file descriptors
        for (int i = sfd + 1; i <= fd_max && fd_count > 0; i++) {
            // check if the fd is in the read pool
            if (FD_ISSET(i, &tmp_r_mask)) {
                fd_count--;
                char buf[256];
                memset(buf, 0, strlen(buf));
                ssize_t r_count = recv(i, &buf, sizeof(buf), 0);
                if (r_count < 0) {
                    if (errno == EAGAIN || errno == EWOULDBLOCK) {
                        log_warn("client is not ready to send data: %s", strerror(errno));
                        break;
                    } else {
                        log_error("failed to receive the data: %s", strerror(errno));
                        exit(1);
                    }
                } else if (r_count == 0) {
                    log_info("received 0 bytes, client shutdown!");
                    if (awaiting == i) {
                        awaiting = 0;
                    }
                    FD_CLR(i, &r_mask);
                    break;
                } else {
                    // TODO handle cases where we've only received some data!
                    char *msg = malloc(sizeof(char) * r_count);
                    strcpy(msg, buf);
                    log_info("received: %s (total of %d bytes received)", msg, r_count);
                    // here we'll store the client id
                    uint c_id = 0;
                    // see if it's the first request to setup the game
                    if (!strcmp(msg, BEGIN_EVENT)) {
                        // find first available client spot and id
                        // reserve 0 id to be left over
                        for (uint j = 1; j < MAX_CLIENTS; j++) {
                            if (!clients[j].id) {
                                c_id = j;
                                break;
                            }
                        }
                        if (!c_id) {
                            log_error("max clients exceeded!");
                            free(msg);
                            continue;
                        }
                        clients[c_id].id = c_id;
                        clients[c_id].fd = i;
                        /*
                         * lets assign the fd to awaiting
                         * it's safe to do it this way because the client will block on recv() until a match is found
                         * so the fd will be the same, unless it crashes in which case we reset the 'awaiting'
                        */
                        if (!awaiting) {
                            clients[c_id].color = WHITE_COLOR;
                            awaiting = c_id;
                        } else {
                            clients[c_id].color = BLACK_COLOR;
                            clients[c_id].opponent_id = awaiting;
                            clients[awaiting].opponent_id = c_id;

                            // set the write masks for both of them, they've been paired, we're ready to respond
                            FD_SET(i, &w_mask);
                            FD_SET(clients[awaiting].fd, &w_mask);

                            // lets pair some more matches
                            awaiting = 0;
                        }
                        // set handshake response with <ID>:<COLOR> format
                        snprintf(clients[c_id].response, 32, "%d:%s", c_id, clients[c_id].color);

                    } else {
                        // lets make sure we're not getting some bad data
                        if (strstr(msg, ":") == NULL) {
                            log_error("invalid msg received! Expected <ID>:<MOVE>");
                            free(msg);
                            continue;
                        }
                        // extract the id and move from the message
                        char *token, *str, *tmp;
                        tmp = str = strdup(msg);
                        token = strsep(&str, ":");
                        char id_str[strlen(token) + 1];
                        strcpy(id_str, token);
                        char *ptr;
                        long l_id = strtol(id_str, &ptr, 10);
                        if (strlen(ptr) > 0 || l_id < 0) {
                            log_error("id is not an integer!");
                            free(msg);
                            free(tmp);
                            continue;
                        }
                        c_id = (uint) l_id;
                        if (c_id >= MAX_CLIENTS) {
                            log_error("id out of bands: %d... we've got a hacker here! He's not one of us!", c_id);
                            free(msg);
                            free(tmp);
                            continue;
                        }
                        token = strsep(&str, ":");
                        // set response for the opponent
                        if (!strcmp(token, END_EVENT)) {
                            // reset to the zero values
                            clients[c_id] = empty_client_t;
                            // free all masks
                            FD_CLR(i, &w_mask);
                            FD_CLR(i, &r_mask);
                            log_info("game between %d and %d was ended!", c_id, clients[c_id].opponent_id);
                        } else if (!strcmp(token, RECONNECT_EVENT)) {
                            // that's all we need to do in order to reestablish the connection
                            // bind the fd to the client and set the write mask for him
                            clients[c_id].fd = i;
                            FD_SET(i, &w_mask);
                        } else {
                            strcpy(clients[clients[c_id].opponent_id].response, token);
                            free(tmp);

                            // make sure the client has already sent BEGIN handshake
                            if (!clients[c_id].id) {
                                log_error("client with id %d was not found!");
                                free(msg);
                                continue;
                            }
                            // update current fd for the connection
                            clients[c_id].fd = i;
                            // set the write mask as we're ready to forward the move
                            FD_SET(clients[clients[c_id].opponent_id].fd, &w_mask);
                        }
                    }
                    free(msg);
                }
            }

            // check if we should respond and if the fd is in the write pool
            if (FD_ISSET(i, &tmp_w_mask)) {
                // lets make sure we can respond already
                uint c_id;
                for (uint j = 1; j < MAX_CLIENTS; j++) {
                    if (clients[j].fd == i) {
                        c_id = j;
                    }
                }
                if (!c_id) {
                    log_warn("we can't respond to a client which doesn't exist!");
                    continue;
                }
                if (!strcmp(clients[c_id].response, "")) {
//                    log_info("response is not yet ready to be emitted for the client: %d", c_id);
                    continue;
                }

                // actual writing to fd logic
                fd_count--;
                ssize_t s_count = send(i, clients[c_id].response, strlen(clients[c_id].response), 0);
                if (s_count < 0) {
                    if (errno == EAGAIN || errno == EWOULDBLOCK) {
                        log_warn("client is not ready to receive data: %s", strerror(errno));
                        continue;
                    } else {
                        log_error("failed to send the data: %s", strerror(errno));
                        continue;
                    }
                } else if (s_count == 0) {
                    log_warn("sent 0 bytes, client can't accept data right now!");
                    continue;
                } else {
                    // TODO handle cases where we've only sent some data!
                    log_info("sent: %s (total of %d bytes sent)", clients[c_id].response, s_count);
                    memset(&clients[c_id].response[0], 0, sizeof(clients[c_id].response));
                    clients[c_id].sent_ctr++;
                }
                if (awaiting == c_id) {
                    awaiting = 0;
                }
                FD_CLR(i, &w_mask);
            }

            // get rid of all the closed write connections
            while (fd_max > sfd && !FD_ISSET(fd_max, &w_mask) && !FD_ISSET(fd_max, &r_mask)) {
                fd_max--;
            }
        }
    }
}

